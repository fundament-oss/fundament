package defclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/install/v1/installv1connect"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/defclient"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/identity"
)

type stubInstall struct {
	installv1connect.UnimplementedInstallServiceHandler
	gotOrganization string
	gotPlugin       string
	gotVersion      string
	gotAuth         string
}

func (s *stubInstall) GetPluginDefinition(
	ctx context.Context,
	req *catalogv1.GetPluginDefinitionRequest,
) (*catalogv1.GetPluginDefinitionResponse, error) {
	callInfo, _ := connect.CallInfoForHandlerContext(ctx)
	s.gotAuth = callInfo.RequestHeader().Get("Authorization")
	s.gotOrganization = req.GetName().GetOrganizationName()
	s.gotPlugin = req.GetName().GetPluginName()
	s.gotVersion = req.GetVersion()
	return catalogv1.GetPluginDefinitionResponse_builder{
		Manifest: []byte("manifest-bytes"), DefinitionHash: "sha256:abc",
	}.Build(), nil
}

func newStubServer(t *testing.T, stub *stubInstall) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	path, h := installv1connect.NewInstallServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGetDefinition(t *testing.T) {
	stub := &stubInstall{}
	srv := newStubServer(t, stub)
	def, err := defclient.New(srv.URL, http.DefaultClient, identity.StaticTokenSource("wt")).
		GetDefinition(t.Context(), "acme", "cert-manager", "v1")
	require.NoError(t, err)
	assert.Equal(t, []byte("manifest-bytes"), def.Manifest)
	assert.Equal(t, "sha256:abc", def.Hash)
	assert.Equal(t, "Bearer wt", stub.gotAuth, "every call carries the WorkloadToken")
}

// The install surface identifies a listing by (organization, plugin) name or
// by id; a PluginInstallation only ever carries the names, so this must send those.
func TestGetDefinition_SendsNameLookup(t *testing.T) {
	stub := &stubInstall{}
	srv := newStubServer(t, stub)
	_, err := defclient.New(srv.URL, srv.Client(), identity.StaticTokenSource("wt")).
		GetDefinition(t.Context(), "acme", "cert-manager", "v1")
	require.NoError(t, err)
	assert.Equal(t, "acme", stub.gotOrganization)
	assert.Equal(t, "cert-manager", stub.gotPlugin)
	assert.Equal(t, "v1", stub.gotVersion)
}

type failingSource struct{}

func (failingSource) Token(context.Context) (string, error) {
	return "", errors.New("exchange workload token: invalid workload credential")
}

// An exchange failure surfaces as the fetch error the reconciler already
// requeues on; no request reaches the install surface.
func TestGetDefinition_ExchangeFailureIsFetchError(t *testing.T) {
	stub := &stubInstall{}
	srv := newStubServer(t, stub)
	_, err := defclient.New(srv.URL, srv.Client(), failingSource{}).
		GetDefinition(t.Context(), "acme", "cert-manager", "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid workload credential")
	assert.Empty(t, stub.gotAuth, "nothing was sent")
}
