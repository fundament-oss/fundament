package authn

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

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
			PluginName:     "cert-manager",
			PluginVersion:  "v1.17.2",
			DefinitionHash: "sha256:1f3c9a",
			OrganizationId: "00000000-0000-0000-0000-000000000abc",
			Status:         "Running",
		}.Build(),
	})

	manifest, err := lookup.GetInstallationManifest(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("GetInstallationManifest: %v", err)
	}
	if manifest.PluginName != "cert-manager" {
		t.Errorf("plugin_name = %q", manifest.PluginName)
	}
	if manifest.PluginVersion != "v1.17.2" {
		t.Errorf("plugin_version = %q", manifest.PluginVersion)
	}
	if manifest.DefinitionHash != "sha256:1f3c9a" {
		t.Errorf("definition_hash = %q", manifest.DefinitionHash)
	}
}

func TestPluginProxyLookup_NotFoundMapsToSentinel(t *testing.T) {
	lookup := NewPluginProxyLookup(&fakeProxyClient{
		err: connect.NewError(connect.CodeNotFound, errors.New("no such installation")),
	})

	_, err := lookup.GetInstallationManifest(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInstallationNotFound) {
		t.Errorf("err = %v, want ErrInstallationNotFound", err)
	}
}

func TestPluginProxyLookup_OtherErrorIsNotSentinel(t *testing.T) {
	lookup := NewPluginProxyLookup(&fakeProxyClient{
		err: connect.NewError(connect.CodeUnavailable, errors.New("plugin-proxy down")),
	})

	_, err := lookup.GetInstallationManifest(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrInstallationNotFound) {
		t.Errorf("transport error %v should not map to a sentinel", err)
	}
}
