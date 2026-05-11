package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteService_CreateSite_InvalidInput(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewSiteServiceClient(env.server.Client(), env.server.URL)

	_, err := client.CreateSite(context.Background(), connect.NewRequest(
		(&dcimv1.CreateSiteRequest_builder{Name: ""}).Build(),
	))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestSiteService_CreateSite(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewSiteServiceClient(env.server.Client(), env.server.URL)

	resp, err := client.CreateSite(context.Background(), connect.NewRequest(
		(&dcimv1.CreateSiteRequest_builder{Name: "Site A"}).Build(),
	))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.GetSiteId())
}
