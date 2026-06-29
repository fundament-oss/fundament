package authn

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pluginproxyv1 "github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
)

// fakeProxyClient returns canned responses so pluginProxyLookup's error
// mapping can be tested without a real plugin-proxy.
type fakeProxyClient struct {
	resp *pluginproxyv1.GetInstallationManifestResponse
	err  error
}

var _ pluginproxyv1connect.PluginInstallationServiceClient = (*fakeProxyClient)(nil)

func (f *fakeProxyClient) GetInstallationManifest(
	_ context.Context,
	_ *connect.Request[pluginproxyv1.GetInstallationManifestRequest],
) (*connect.Response[pluginproxyv1.GetInstallationManifestResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(f.resp), nil
}

func TestPluginProxyLookup_Success(t *testing.T) {
	lookup := NewPluginProxyLookup(&fakeProxyClient{
		resp: pluginproxyv1.GetInstallationManifestResponse_builder{
			PluginName:     testPluginName,
			PluginVersion:  "v1.17.2",
			DefinitionHash: "sha256:1f3c9a",
			OrganizationId: "00000000-0000-0000-0000-000000000abc",
			Status:         "Running",
		}.Build(),
	})

	manifest, err := lookup.GetInstallationManifest(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err, "GetInstallationManifest")
	assert.Equal(t, testPluginName, manifest.PluginName)
	assert.Equal(t, "v1.17.2", manifest.PluginVersion)
	assert.Equal(t, "sha256:1f3c9a", manifest.DefinitionHash)
}

func TestPluginProxyLookup_NotFoundMapsToSentinel(t *testing.T) {
	lookup := NewPluginProxyLookup(&fakeProxyClient{
		err: connect.NewError(connect.CodeNotFound, errors.New("no such installation")),
	})

	_, err := lookup.GetInstallationManifest(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, ErrInstallationNotFound)
}

func TestPluginProxyLookup_OtherErrorIsNotSentinel(t *testing.T) {
	lookup := NewPluginProxyLookup(&fakeProxyClient{
		err: connect.NewError(connect.CodeUnavailable, errors.New("plugin-proxy down")),
	})

	_, err := lookup.GetInstallationManifest(context.Background(), uuid.New(), uuid.New())
	require.Error(t, err, "expected error")
	assert.NotErrorIs(t, err, ErrInstallationNotFound, "transport error should not map to a sentinel")
}
