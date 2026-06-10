package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
	"github.com/stretchr/testify/require"
)

func TestRackService_DeleteRack_HappyFlow(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.client(), env.server.URL)

	rowID := createRackRowFixture(t, env, "Rack Delete")
	rackID := createRack(t, env, rowID, "Rack To Delete", 24)

	_, err := client.DeleteRack(context.Background(), connect.NewRequest(
		(&dcimv1.DeleteRackRequest_builder{Id: rackID}).Build(),
	))
	require.NoError(t, err)

	_, err = client.GetRack(context.Background(), connect.NewRequest(
		(&dcimv1.GetRackRequest_builder{Id: rackID}).Build(),
	))
	requireCode(t, err, connect.CodeNotFound)
}

func TestRackService_DeleteRack_Errors(t *testing.T) {
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
			_, err := client.DeleteRack(context.Background(), connect.NewRequest(
				(&dcimv1.DeleteRackRequest_builder{Id: tc.id}).Build(),
			))
			requireCode(t, err, tc.want)
		})
	}
}
