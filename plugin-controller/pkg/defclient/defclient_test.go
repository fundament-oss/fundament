package defclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/defclient"
)

type stubPlugin struct {
	organizationv1connect.UnimplementedPluginServiceHandler
	gotOrganization string
	gotPlugin       string
}

func (s *stubPlugin) GetPluginDefinition(_ context.Context, req *organizationv1.GetPluginDefinitionRequest) (*organizationv1.GetPluginDefinitionResponse, error) {
	s.gotOrganization = req.GetOrganizationName()
	s.gotPlugin = req.GetPluginName()
	return organizationv1.GetPluginDefinitionResponse_builder{
		Manifest: []byte("manifest-bytes"), Hash: "sha256:abc",
	}.Build(), nil
}

func TestGetDefinition(t *testing.T) {
	mux := http.NewServeMux()
	path, h := organizationv1connect.NewPluginServiceHandler(&stubPlugin{})
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := defclient.New(srv.URL, http.DefaultClient)
	def, err := c.GetDefinition(context.Background(), "acme", "cert-manager", "v1")
	require.NoError(t, err)
	assert.Equal(t, []byte("manifest-bytes"), def.Manifest)
	assert.Equal(t, "sha256:abc", def.Hash)
}

func TestGetDefinition_SendsOrganizationName(t *testing.T) {
	stub := &stubPlugin{}
	path, h := organizationv1connect.NewPluginServiceHandler(stub)
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := defclient.New(srv.URL, srv.Client()).GetDefinition(t.Context(), "acme", "cert-manager", "v1")
	require.NoError(t, err)
	assert.Equal(t, "sha256:abc", got.Hash)
	assert.Equal(t, "acme", stub.gotOrganization)
	assert.Equal(t, "cert-manager", stub.gotPlugin)
}
