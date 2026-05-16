package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
)

func TestRackService_CreateRack(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.server.Client(), env.server.URL)

	tests := []struct {
		name       string
		rowID      string
		rackName   string
		totalUnits int32
		want       connect.Code
	}{
		{"missing_row_id", "", "Rack A", 42, connect.CodeInvalidArgument},
		{"invalid_row_id", invalidUUID, "Rack A", 42, connect.CodeInvalidArgument},
		{"empty_name", validUUID, "", 42, connect.CodeInvalidArgument},
		{"zero_total_units", validUUID, "Rack A", 0, connect.CodeInvalidArgument},
		{"negative_total_units", validUUID, "Rack A", -1, connect.CodeInvalidArgument},
		{"row_not_found", validUUID, "Rack A", 42, connect.CodeNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := client.CreateRack(context.Background(), connect.NewRequest(
				(&dcimv1.CreateRackRequest_builder{
					RowId:      tc.rowID,
					Name:       tc.rackName,
					TotalUnits: tc.totalUnits,
				}).Build(),
			))
			requireCode(t, err, tc.want)
		})
	}
}
