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

func TestAssetService_GetAssetLocation_Rack(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewAssetServiceClient(env.client(), env.server.URL)

	siteID := createSite(t, env, "Loc site")
	roomID := createRoom(t, env, siteID, "Loc room")
	rowID := createRackRow(t, env, roomID, "Loc row")
	rackID := createRack(t, env, rowID, "Loc rack", 42)

	catalogID := createCatalogEntry(t, env, "Loc model")
	assetID := createAsset(t, env, catalogID)
	placeAssetInRack(t, env, assetID, rackID, 5)

	resp, err := client.GetAssetLocation(context.Background(), connect.NewRequest(
		(&dcimv1.GetAssetLocationRequest_builder{AssetId: assetID}).Build(),
	))
	require.NoError(t, err)

	loc := resp.Msg.GetLocation()
	require.NotNil(t, loc)
	assert.Equal(t, "Loc site", loc.GetSiteName())
	assert.Equal(t, "Loc room", loc.GetRoomName())
	assert.Equal(t, "Loc row", loc.GetRackRowName())
	assert.Equal(t, "Loc rack", loc.GetRackName())
	assert.Equal(t, rackID, loc.GetRackId())
	assert.Equal(t, int32(5), loc.GetRackUnitStart())
	assert.Equal(t, dcimv1.RackSlotType_RACK_SLOT_TYPE_UNIT, loc.GetRackSlotType())
}

// A sub-component (placement with parent_placement_id and no rack_id) must
// resolve to the rack of its top-level host via the recursive CTE.
func TestAssetService_GetAssetLocation_SubComponentResolvesToHostRack(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewAssetServiceClient(env.client(), env.server.URL)

	siteID := createSite(t, env, "Nested site")
	roomID := createRoom(t, env, siteID, "Nested room")
	rowID := createRackRow(t, env, roomID, "Nested row")
	rackID := createRack(t, env, rowID, "Nested rack", 42)

	hostCatalogID := createCatalogEntry(t, env, "Host model")
	hostAssetID := createAsset(t, env, hostCatalogID)
	hostPlacementID := placeAssetInRack(t, env, hostAssetID, rackID, 10)

	// Define a port on the host so the sub-component placement has a real
	// parent_port_definition_id to point at.
	catalogClient := dcimv1connect.NewCatalogServiceClient(env.client(), env.server.URL)
	portResp, err := catalogClient.CreatePortDefinition(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePortDefinitionRequest_builder{
			DeviceCatalogId: hostCatalogID,
			Name:            "slot0",
			PortType:        dcimv1.PortType_PORT_TYPE_SLOT,
			Direction:       dcimv1.PortDirection_PORT_DIRECTION_BIDIR,
		}).Build(),
	))
	require.NoError(t, err)

	componentCatalogID := createCatalogEntry(t, env, "Component model")
	componentAssetID := createAsset(t, env, componentCatalogID)
	placeAssetInSubComponent(t, env, componentAssetID, hostPlacementID, portResp.Msg.GetPortDefinitionId())

	resp, err := client.GetAssetLocation(context.Background(), connect.NewRequest(
		(&dcimv1.GetAssetLocationRequest_builder{AssetId: componentAssetID}).Build(),
	))
	require.NoError(t, err)

	loc := resp.Msg.GetLocation()
	require.NotNil(t, loc)
	assert.Equal(t, "Nested rack", loc.GetRackName())
	assert.Equal(t, rackID, loc.GetRackId())
	assert.Equal(t, int32(10), loc.GetRackUnitStart())
}

func TestAssetService_GetAssetLocation_Unplaced(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewAssetServiceClient(env.client(), env.server.URL)

	catalogID := createCatalogEntry(t, env, "Unplaced model")
	assetID := createAsset(t, env, catalogID)

	resp, err := client.GetAssetLocation(context.Background(), connect.NewRequest(
		(&dcimv1.GetAssetLocationRequest_builder{AssetId: assetID}).Build(),
	))
	require.NoError(t, err)

	// No location means the rack id is empty.
	assert.Empty(t, resp.Msg.GetLocation().GetRackId())
}
