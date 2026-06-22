package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
)

// TestPlacementService_CreatePlacement_RejectsUnitBelowOne guards against
// off-grid rack placements: racks are numbered from 1, so a rack_unit_start of
// 0 would render in no slot. The RackLocation validation must reject it.
func TestPlacementService_CreatePlacement_RejectsUnitBelowOne(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewPlacementServiceClient(env.client(), env.server.URL)

	rowID := createRackRowFixture(t, env, "Unit")
	rackID := createRack(t, env, rowID, "Unit rack", 42)
	catalogID := createCatalogEntry(t, env, "Unit model")
	assetID := createAsset(t, env, catalogID)

	_, err := client.CreatePlacement(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePlacementRequest_builder{
			AssetId: assetID,
			Rack: (&dcimv1.RackLocation_builder{
				RackId:        rackID,
				RackUnitStart: 0,
				RackSlotType:  dcimv1.RackSlotType_RACK_SLOT_TYPE_UNIT,
			}).Build(),
		}).Build(),
	))
	requireCode(t, err, connect.CodeInvalidArgument)
}
