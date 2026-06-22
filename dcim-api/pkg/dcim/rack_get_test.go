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

func TestRackService_GetRack_HappyFlow(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.client(), env.server.URL)

	rowID := createRackRowFixture(t, env, "Rack Get")
	rackID := createRack(t, env, rowID, "Rack Get Target", 24)

	resp, err := client.GetRack(context.Background(), connect.NewRequest(
		(&dcimv1.GetRackRequest_builder{Id: rackID}).Build(),
	))
	require.NoError(t, err)

	rack := resp.Msg.GetRack()
	require.NotNil(t, rack)
	assert.Equal(t, rackID, rack.GetId())
	assert.Equal(t, rowID, rack.GetRowId())
	assert.Equal(t, "Rack Get Target", rack.GetName())
	assert.Equal(t, int32(24), rack.GetTotalUnits())
}

func TestRackService_GetRack_Errors(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.client(), env.server.URL)

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
			_, err := client.GetRack(context.Background(), connect.NewRequest(
				(&dcimv1.GetRackRequest_builder{Id: tc.id}).Build(),
			))
			requireCode(t, err, tc.want)
		})
	}
}
