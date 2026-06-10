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

func TestSiteService_UpdateSite_HappyFlow(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewSiteServiceClient(env.client(), env.server.URL)

	siteID := createSite(t, env, "Site Before Update")

	newName := "Site After Update"
	newAddress := "Updated street 1"
	_, err := client.UpdateSite(context.Background(), connect.NewRequest(
		(&dcimv1.UpdateSiteRequest_builder{
			Id:      siteID,
			Name:    &newName,
			Address: &newAddress,
		}).Build(),
	))
	require.NoError(t, err)

	getResp, err := client.GetSite(context.Background(), connect.NewRequest(
		(&dcimv1.GetSiteRequest_builder{Id: siteID}).Build(),
	))
	require.NoError(t, err)
	site := getResp.Msg.GetSite()
	require.NotNil(t, site)
	assert.Equal(t, newName, site.GetName())
	assert.Equal(t, newAddress, site.GetAddress())
}

func TestSiteService_UpdateSite(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewSiteServiceClient(env.client(), env.server.URL)

	tests := []struct {
		name string
		id   string
		want connect.Code
	}{
		{"empty_id", "", connect.CodeInvalidArgument},
		{"invalid_uuid", invalidUUID, connect.CodeInvalidArgument},
		{"not_found", validUUID, connect.CodeNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := client.UpdateSite(context.Background(), connect.NewRequest(
				(&dcimv1.UpdateSiteRequest_builder{Id: tc.id}).Build(),
			))
			requireCode(t, err, tc.want)
		})
	}
}
