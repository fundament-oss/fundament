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

func TestRackService_UpdateRack_HappyFlow(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.server.Client(), env.server.URL)

	rowID := createRackRowFixture(t, env, "Rack Update")
	rackID := createRack(t, env, rowID, "Rack Before Update", 24)

	newName := "Rack After Update"
	newUnits := int32(48)
	_, err := client.UpdateRack(context.Background(), connect.NewRequest(
		(&dcimv1.UpdateRackRequest_builder{
			Id:         rackID,
			Name:       &newName,
			TotalUnits: &newUnits,
		}).Build(),
	))
	require.NoError(t, err)

	getResp, err := client.GetRack(context.Background(), connect.NewRequest(
		(&dcimv1.GetRackRequest_builder{Id: rackID}).Build(),
	))
	require.NoError(t, err)

	rack := getResp.Msg.GetRack()
	require.NotNil(t, rack)

	assert.Equal(t, newName, rack.GetName())
	assert.Equal(t, newUnits, rack.GetTotalUnits())
}

func TestRackService_UpdateRack_Errors(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.server.Client(), env.server.URL)

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
			_, err := client.UpdateRack(context.Background(), connect.NewRequest(
				(&dcimv1.UpdateRackRequest_builder{Id: tc.id}).Build(),
			))
			requireCode(t, err, tc.want)
		})
	}
}
