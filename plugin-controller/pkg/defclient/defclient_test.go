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
}

func (stubPlugin) GetPluginDefinition(_ context.Context, _ *organizationv1.GetPluginDefinitionRequest) (*organizationv1.GetPluginDefinitionResponse, error) {
	return organizationv1.GetPluginDefinitionResponse_builder{
		Manifest: []byte("manifest-bytes"), Hash: "sha256:abc",
	}.Build(), nil
}

func TestGetDefinition(t *testing.T) {
	mux := http.NewServeMux()
	path, h := organizationv1connect.NewPluginServiceHandler(stubPlugin{})
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := defclient.New(srv.URL, http.DefaultClient)
	def, err := c.GetDefinition(context.Background(), "cert-manager", "v1")
	require.NoError(t, err)
	assert.Equal(t, []byte("manifest-bytes"), def.Manifest)
	assert.Equal(t, "sha256:abc", def.Hash)
}
