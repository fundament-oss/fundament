package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
)

func TestRackService_GetRack(t *testing.T) {
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
			_, err := client.GetRack(context.Background(), connect.NewRequest(
				(&dcimv1.GetRackRequest_builder{Id: tc.id}).Build(),
			))
			requireCode(t, err, tc.want)
		})
	}
}
