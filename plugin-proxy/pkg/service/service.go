// Package service implements PluginInstallationService: a CR lookup on a
// target cluster returning the installation's identity for authn-api to sign
// into a PluginToken. The response carries no RBAC.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	pluginproxyv1 "github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
	"github.com/google/uuid"
)

// ClusterAccess resolves a cluster ID to the data plugin-proxy needs to read
// PluginInstallation CRs: a controller-runtime client for the target cluster
// and the owning organization. The mock implementation lives in mock.go; a
// real (Gardener-backed) implementation is future work.
type ClusterAccess interface {
	ForCluster(ctx context.Context, clusterID uuid.UUID) (*ClusterTarget, error)
}

// ClusterTarget is what ClusterAccess returns for a known cluster.
type ClusterTarget struct {
	Client         client.Client
	OrganizationID uuid.UUID
}

// Service is the PluginInstallationService Connect handler.
type Service struct {
	Logger  *slog.Logger
	Cluster ClusterAccess
}

var _ pluginproxyv1connect.PluginInstallationServiceHandler = (*Service)(nil)

// New constructs a Service.
func New(logger *slog.Logger, cluster ClusterAccess) *Service {
	return &Service{Logger: logger, Cluster: cluster}
}

// GetInstallationManifest returns the installation's identity. A missing
// installation maps to NotFound; an unknown cluster maps to Unavailable.
// All lifecycle phases (including Terminating) return the manifest — plugin
// tokens are also used to read state during teardown.
func (s *Service) GetInstallationManifest(
	ctx context.Context,
	req *connect.Request[pluginproxyv1.GetInstallationManifestRequest],
) (*connect.Response[pluginproxyv1.GetInstallationManifestResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())
	installationID := uuid.MustParse(req.Msg.GetInstallationId())

	target, err := s.Cluster.ForCluster(ctx, clusterID)
	if err != nil {
		s.Logger.Debug("cluster lookup failed", "cluster_id", clusterID, "error", err)
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("cluster lookup: %w", err))
	}

	found, err := s.findInstallation(ctx, target.Client, installationID)
	if err != nil {
		return nil, err
	}

	phase := string(found.Status.Phase)
	s.Logger.Debug("installation resolved",
		"cluster_id", clusterID,
		"installation_id", installationID,
		"plugin_name", found.Spec.DefinitionRef.PluginName,
		"plugin_version", found.Spec.DefinitionRef.PluginVersion,
		"phase", phase,
	)

	resp := pluginproxyv1.GetInstallationManifestResponse_builder{
		PluginName:     found.Spec.DefinitionRef.PluginName,
		PluginVersion:  found.Spec.DefinitionRef.PluginVersion,
		DefinitionHash: found.Spec.DefinitionRef.DefinitionHash,
		OrganizationId: target.OrganizationID.String(),
		Status:         phase,
	}.Build()
	return connect.NewResponse(resp), nil
}

// findInstallation locates a PluginInstallation by UID. A missing
// installation is reported as NotFound.
func (s *Service) findInstallation(
	ctx context.Context, c client.Client, installationID uuid.UUID,
) (*pluginsv1.PluginInstallation, error) {
	var list pluginsv1.PluginInstallationList
	if err := c.List(ctx, &list); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("list installations: %w", err))
	}
	for i := range list.Items {
		if strings.Compare(string(list.Items[i].UID), installationID.String()) == 0 {
			return &list.Items[i], nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound,
		fmt.Errorf("installation %q not found", installationID))
}
