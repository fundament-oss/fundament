package defclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
	"github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1/catalogv1connect"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/defclient"
)

type stubCatalog struct {
	catalogv1connect.UnimplementedCatalogServiceHandler
	gotOrganization string
	gotPlugin       string
	gotVersion      string
}

func (s *stubCatalog) GetPluginDefinition(
	_ context.Context,
	req *catalogv1.GetPluginDefinitionRequest,
) (*catalogv1.GetPluginDefinitionResponse, error) {
	s.gotOrganization = req.GetName().GetOrganizationName()
	s.gotPlugin = req.GetName().GetPluginName()
	s.gotVersion = req.GetVersion()

	return catalogv1.GetPluginDefinitionResponse_builder{
		Manifest: []byte("manifest-bytes"), DefinitionHash: "sha256:abc",
	}.Build(), nil
}

func newStubServer(t *testing.T, stub *stubCatalog) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	path, h := catalogv1connect.NewCatalogServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func TestGetDefinition(t *testing.T) {
	srv := newStubServer(t, &stubCatalog{})

	def, err := defclient.New(srv.URL, http.DefaultClient).
		GetDefinition(t.Context(), "acme", "cert-manager", "v1")
	require.NoError(t, err)

	assert.Equal(t, []byte("manifest-bytes"), def.Manifest)
	assert.Equal(t, "sha256:abc", def.Hash)
}

// The catalog identifies a listing by (organization, plugin) name or by id; a
// PluginInstallation only ever carries the names, so this must send those.
func TestGetDefinition_SendsNameLookup(t *testing.T) {
	stub := &stubCatalog{}
	srv := newStubServer(t, stub)

	_, err := defclient.New(srv.URL, srv.Client()).
		GetDefinition(t.Context(), "acme", "cert-manager", "v1")
	require.NoError(t, err)

	assert.Equal(t, "acme", stub.gotOrganization)
	assert.Equal(t, "cert-manager", stub.gotPlugin)
	assert.Equal(t, "v1", stub.gotVersion)
}
