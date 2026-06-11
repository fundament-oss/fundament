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

// InstallationManifest is the identity slice plugin-proxy returns to authn-api
// at mint time. It deliberately carries no RBAC: per FUN-17 the plugin's scope
// lives in the immutable PluginDefinition and is materialised into a
// Kubernetes Role by plugin-controller. authn-api signs these fields into the
// PluginToken.
type InstallationManifest struct {
	PluginName     string
	PluginVersion  string
	DefinitionHash string
}

// ErrInstallationNotFound signals that no PluginInstallation with the given
// (cluster, installation) tuple is known. The mint handler maps it to
// connect.CodeNotFound.
var ErrInstallationNotFound = errors.New("plugin installation not found")

// ErrInstallationTerminating signals that the PluginInstallation exists but is
// being torn down (its CR carries a deletionTimestamp). The mint handler maps
// it to connect.CodeFailedPrecondition.
var ErrInstallationTerminating = errors.New("plugin installation is terminating")

// PluginInstallationLookup resolves a plugin installation's identity by
// (cluster, installation). authn-api calls this at every MintPluginToken
// request.
//
// In production it is implemented as a Connect client against
// plugin-proxy.PluginInstallationService/GetInstallationManifest. Tests inject
// fakes.
type PluginInstallationLookup interface {
	GetInstallationManifest(ctx context.Context, clusterID, installationID uuid.UUID) (*InstallationManifest, error)
}

// pluginProxyLookup is the production PluginInstallationLookup. It calls
// plugin-proxy's internal PluginInstallationService over Connect and maps its
// error codes onto this package's sentinel errors.
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
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			return nil, ErrInstallationNotFound
		case connect.CodeFailedPrecondition:
			return nil, ErrInstallationTerminating
		default:
			return nil, fmt.Errorf("plugin-proxy get installation manifest: %w", err)
		}
	}

	msg := resp.Msg
	return &InstallationManifest{
		PluginName:     msg.GetPluginName(),
		PluginVersion:  msg.GetPluginVersion(),
		DefinitionHash: msg.GetDefinitionHash(),
	}, nil
}
