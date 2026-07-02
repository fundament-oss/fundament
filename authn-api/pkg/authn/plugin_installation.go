package authn

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pluginproxyv1 "github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
)

// InstallationManifest is the identity slice plugin-proxy returns at mint
// time. authn-api signs these fields into the PluginToken.
type InstallationManifest struct {
	PluginName     string
	PluginVersion  string
	DefinitionHash string
}

// ErrInstallationNotFound is returned by PluginInstallationLookup when no
// installation matches the (cluster, installation) tuple.
var ErrInstallationNotFound = errors.New("plugin installation not found")

// PluginInstallationLookup resolves a plugin installation's identity by
// (cluster, installation). authn-api calls this at every MintPluginToken
// request.
type PluginInstallationLookup interface {
	GetInstallationManifest(ctx context.Context, clusterID, installationID uuid.UUID) (*InstallationManifest, error)
}

type pluginProxyLookup struct {
	client pluginproxyv1connect.PluginInstallationServiceClient
}

// NewPluginProxyLookup returns a PluginInstallationLookup backed by the
// plugin-proxy PluginInstallationService client.
func NewPluginProxyLookup(client pluginproxyv1connect.PluginInstallationServiceClient) PluginInstallationLookup {
	return &pluginProxyLookup{client: client}
}

func (l *pluginProxyLookup) GetInstallationManifest(
	ctx context.Context, clusterID, installationID uuid.UUID,
) (*InstallationManifest, error) {
	resp, err := l.client.GetInstallationManifest(ctx, connect.NewRequest(
		pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      clusterID.String(),
			InstallationId: installationID.String(),
		}.Build()))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, ErrInstallationNotFound
		}
		return nil, fmt.Errorf("plugin-proxy get installation manifest: %w", err)
	}

	msg := resp.Msg
	return &InstallationManifest{
		PluginName:     msg.GetPluginName(),
		PluginVersion:  msg.GetPluginVersion(),
		DefinitionHash: msg.GetDefinitionHash(),
	}, nil
}
