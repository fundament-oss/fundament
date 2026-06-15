// Package installation implements the internal PluginInstallationService RPC.
// It reads a PluginInstallation CR from a target cluster and returns the
// installation's identity (no RBAC) for authn-api to sign into a PluginToken.
package installation

import (
	"context"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	pluginproxyv1 "github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
)

// ClusterClientFn returns a controller-runtime client for the given cluster.
type ClusterClientFn func(ctx context.Context, clusterID string) (client.Client, error)

// OrgIDForClusterFn resolves a cluster ID to its owning organization ID.
type OrgIDForClusterFn func(ctx context.Context, clusterID string) (string, error)

// Service is the PluginInstallationService Connect handler.
type Service struct {
	Logger          *slog.Logger
	ClusterClient   ClusterClientFn
	OrgIDForCluster OrgIDForClusterFn
}

var _ pluginproxyv1connect.PluginInstallationServiceHandler = (*Service)(nil)

// NewService constructs a Service and rejects a nil ClusterClient or
// OrgIDForCluster at startup. Without this, a misconfigured wiring surfaces
// as a runtime nil-call panic at first traffic — recovered to a 500 by the
// recovery interceptor, but the cause is invisible from the outside.
func NewService(logger *slog.Logger, clusterClient ClusterClientFn, orgIDForCluster OrgIDForClusterFn) *Service {
	if clusterClient == nil {
		panic("installation.NewService: clusterClient is nil")
	}
	if orgIDForCluster == nil {
		panic("installation.NewService: orgIDForCluster is nil")
	}
	return &Service{
		Logger:          logger,
		ClusterClient:   clusterClient,
		OrgIDForCluster: orgIDForCluster,
	}
}

// GetInstallationManifest resolves a PluginInstallation's identity from its
// spec.definitionRef. It returns NotFound when the installation does not
// exist and FailedPrecondition when it is terminating; per FUN-17 it never
// returns RBAC.
func (s *Service) GetInstallationManifest(
	ctx context.Context,
	req *connect.Request[pluginproxyv1.GetInstallationManifestRequest],
) (*connect.Response[pluginproxyv1.GetInstallationManifestResponse], error) {
	clusterID := req.Msg.GetClusterId()
	installationID := req.Msg.GetInstallationId()

	c, err := s.ClusterClient(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("resolve cluster client: %w", err))
	}

	found, err := s.findInstallation(ctx, c, installationID)
	if err != nil {
		return nil, err
	}

	// Refuse a mint for an installation being torn down — either the CR
	// already carries a deletionTimestamp, or the controller has moved it to
	// the Terminating phase. authn-api maps this to FailedPrecondition
	// (FUN-17 Lifecycle).
	if found.DeletionTimestamp != nil || found.Status.Phase == pluginsv1.PluginPhaseTerminating {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("installation %q is terminating", installationID))
	}

	orgID, err := s.OrgIDForCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("resolve org id: %w", err))
	}

	resp := pluginproxyv1.GetInstallationManifestResponse_builder{
		PluginName:     found.Spec.DefinitionRef.PluginName,
		PluginVersion:  found.Spec.DefinitionRef.PluginVersion,
		DefinitionHash: found.Spec.DefinitionRef.DefinitionHash,
		OrganizationId: orgID,
		Status:         string(found.Status.Phase),
	}.Build()
	return connect.NewResponse(resp), nil
}

// findInstallation locates the PluginInstallation by UID, falling back to a
// Get by name for fixtures where UID == name. A missing installation is
// reported as a NotFound connect error.
func (s *Service) findInstallation(
	ctx context.Context, c client.Client, installationID string,
) (*pluginsv1.PluginInstallation, error) {
	var list pluginsv1.PluginInstallationList
	if err := c.List(ctx, &list); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("list installations: %w", err))
	}
	for i := range list.Items {
		if string(list.Items[i].UID) == installationID {
			return &list.Items[i], nil
		}
	}

	var direct pluginsv1.PluginInstallation
	if err := c.Get(ctx, types.NamespacedName{Name: installationID}, &direct); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("installation %q not found", installationID))
		}
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("get installation: %w", err))
	}
	return &direct, nil
}
