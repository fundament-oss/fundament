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

// physicalConnectionFixture builds a rack with two placed assets that each
// expose one port, and returns the two (placement id, port definition id) pairs
// so a physical connection can be created between them.
func physicalConnectionFixture(t *testing.T, env *testEnv) (aPlacement, aPort, bPlacement, bPort string) {
	t.Helper()

	rowID := createRackRowFixture(t, env, "Cable")
	rackID := createRack(t, env, rowID, "Cable rack", 42)

	catalogID := createCatalogEntry(t, env, "Cable model")
	aPort = createPortDefinition(t, env, catalogID, "eth0")
	bPort = createPortDefinition(t, env, catalogID, "eth1")

	aAsset := createAsset(t, env, catalogID)
	bAsset := createAsset(t, env, catalogID)
	aPlacement = placeAssetInRack(t, env, aAsset, rackID, 1)
	bPlacement = placeAssetInRack(t, env, bAsset, rackID, 2)

	return aPlacement, aPort, bPlacement, bPort
}

// TestPhysicalConnectionService_CableAttributesRoundTrip verifies that the
// cable presentation attributes (type/status/color/label) survive a create and
// can be changed via update.
func TestPhysicalConnectionService_CableAttributesRoundTrip(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewPhysicalConnectionServiceClient(env.server.Client(), env.server.URL)

	aPlacement, aPort, bPlacement, bPort := physicalConnectionFixture(t, env)

	createResp, err := client.CreatePhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePhysicalConnectionRequest_builder{
			SourcePlacementId:      aPlacement,
			SourcePortDefinitionId: aPort,
			TargetPlacementId:      bPlacement,
			TargetPortDefinitionId: bPort,
			CableType:              dcimv1.CableType_CABLE_TYPE_CAT6A,
			Status:                 dcimv1.CableStatus_CABLE_STATUS_CONNECTED,
			Color:                  dcimv1.CableColor_CABLE_COLOR_BLUE,
			Label:                  "srv01-data",
		}).Build(),
	))
	require.NoError(t, err)
	require.NotEmpty(t, createResp.Msg.GetConnectionId())

	connID := createResp.Msg.GetConnectionId()

	getResp, err := client.GetPhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.GetPhysicalConnectionRequest_builder{Id: connID}).Build(),
	))
	require.NoError(t, err)

	conn := getResp.Msg.GetConnection()
	require.NotNil(t, conn)
	assert.Equal(t, dcimv1.CableType_CABLE_TYPE_CAT6A, conn.GetCableType())
	assert.Equal(t, dcimv1.CableStatus_CABLE_STATUS_CONNECTED, conn.GetStatus())
	assert.Equal(t, dcimv1.CableColor_CABLE_COLOR_BLUE, conn.GetColor())
	assert.Equal(t, "srv01-data", conn.GetLabel())

	// Update every attribute to a different value. Update fields use explicit
	// presence, so the builder takes pointers.
	updatedType := dcimv1.CableType_CABLE_TYPE_DAC
	updatedStatus := dcimv1.CableStatus_CABLE_STATUS_PLANNED
	updatedColor := dcimv1.CableColor_CABLE_COLOR_TEAL
	updatedLabel := "spine-uplink"
	_, err = client.UpdatePhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.UpdatePhysicalConnectionRequest_builder{
			Id:        connID,
			CableType: &updatedType,
			Status:    &updatedStatus,
			Color:     &updatedColor,
			Label:     &updatedLabel,
		}).Build(),
	))
	require.NoError(t, err)

	getResp, err = client.GetPhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.GetPhysicalConnectionRequest_builder{Id: connID}).Build(),
	))
	require.NoError(t, err)

	conn = getResp.Msg.GetConnection()
	require.NotNil(t, conn)
	assert.Equal(t, dcimv1.CableType_CABLE_TYPE_DAC, conn.GetCableType())
	assert.Equal(t, dcimv1.CableStatus_CABLE_STATUS_PLANNED, conn.GetStatus())
	assert.Equal(t, dcimv1.CableColor_CABLE_COLOR_TEAL, conn.GetColor())
	assert.Equal(t, "spine-uplink", conn.GetLabel())
}

// TestPhysicalConnectionService_CableAttributesOptional verifies that a
// connection created without any cable attributes comes back with unspecified
// enums and an empty label, and that a partial update leaves the untouched
// attributes intact.
func TestPhysicalConnectionService_CableAttributesOptional(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewPhysicalConnectionServiceClient(env.server.Client(), env.server.URL)

	aPlacement, aPort, bPlacement, bPort := physicalConnectionFixture(t, env)

	createResp, err := client.CreatePhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePhysicalConnectionRequest_builder{
			SourcePlacementId:      aPlacement,
			SourcePortDefinitionId: aPort,
			TargetPlacementId:      bPlacement,
			TargetPortDefinitionId: bPort,
		}).Build(),
	))
	require.NoError(t, err)

	connID := createResp.Msg.GetConnectionId()

	getResp, err := client.GetPhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.GetPhysicalConnectionRequest_builder{Id: connID}).Build(),
	))
	require.NoError(t, err)

	conn := getResp.Msg.GetConnection()
	require.NotNil(t, conn)
	assert.Equal(t, dcimv1.CableType_CABLE_TYPE_UNSPECIFIED, conn.GetCableType())
	assert.Equal(t, dcimv1.CableStatus_CABLE_STATUS_UNSPECIFIED, conn.GetStatus())
	assert.Equal(t, dcimv1.CableColor_CABLE_COLOR_UNSPECIFIED, conn.GetColor())
	assert.Empty(t, conn.GetLabel())

	// Update only the status; the other attributes must stay unset.
	updatedStatus := dcimv1.CableStatus_CABLE_STATUS_DECOMMISSIONED
	_, err = client.UpdatePhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.UpdatePhysicalConnectionRequest_builder{
			Id:     connID,
			Status: &updatedStatus,
		}).Build(),
	))
	require.NoError(t, err)

	getResp, err = client.GetPhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.GetPhysicalConnectionRequest_builder{Id: connID}).Build(),
	))
	require.NoError(t, err)

	conn = getResp.Msg.GetConnection()
	require.NotNil(t, conn)
	assert.Equal(t, dcimv1.CableStatus_CABLE_STATUS_DECOMMISSIONED, conn.GetStatus())
	assert.Equal(t, dcimv1.CableType_CABLE_TYPE_UNSPECIFIED, conn.GetCableType())
	assert.Equal(t, dcimv1.CableColor_CABLE_COLOR_UNSPECIFIED, conn.GetColor())
}

// TestPhysicalConnectionService_CableAttributesClear verifies that an update
// carrying the empty sentinel (UNSPECIFIED enum / empty label) for an
// explicitly-set field clears the previously-stored value.
func TestPhysicalConnectionService_CableAttributesClear(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewPhysicalConnectionServiceClient(env.server.Client(), env.server.URL)

	aPlacement, aPort, bPlacement, bPort := physicalConnectionFixture(t, env)

	createResp, err := client.CreatePhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePhysicalConnectionRequest_builder{
			SourcePlacementId:      aPlacement,
			SourcePortDefinitionId: aPort,
			TargetPlacementId:      bPlacement,
			TargetPortDefinitionId: bPort,
			CableType:              dcimv1.CableType_CABLE_TYPE_CAT6,
			Status:                 dcimv1.CableStatus_CABLE_STATUS_CONNECTED,
			Color:                  dcimv1.CableColor_CABLE_COLOR_RED,
			Label:                  "to-clear",
		}).Build(),
	))
	require.NoError(t, err)

	connID := createResp.Msg.GetConnectionId()

	// Explicitly set every attribute to its empty sentinel: this must null the
	// columns rather than be ignored.
	clearType := dcimv1.CableType_CABLE_TYPE_UNSPECIFIED
	clearStatus := dcimv1.CableStatus_CABLE_STATUS_UNSPECIFIED
	clearColor := dcimv1.CableColor_CABLE_COLOR_UNSPECIFIED
	clearLabel := ""
	_, err = client.UpdatePhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.UpdatePhysicalConnectionRequest_builder{
			Id:        connID,
			CableType: &clearType,
			Status:    &clearStatus,
			Color:     &clearColor,
			Label:     &clearLabel,
		}).Build(),
	))
	require.NoError(t, err)

	getResp, err := client.GetPhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.GetPhysicalConnectionRequest_builder{Id: connID}).Build(),
	))
	require.NoError(t, err)

	conn := getResp.Msg.GetConnection()
	require.NotNil(t, conn)
	assert.Equal(t, dcimv1.CableType_CABLE_TYPE_UNSPECIFIED, conn.GetCableType())
	assert.Equal(t, dcimv1.CableStatus_CABLE_STATUS_UNSPECIFIED, conn.GetStatus())
	assert.Equal(t, dcimv1.CableColor_CABLE_COLOR_UNSPECIFIED, conn.GetColor())
	assert.Empty(t, conn.GetLabel())
}

// TestPhysicalConnectionService_ListConnectionsBySite verifies that the
// site-scoped list returns every connection touching a rack in the site and
// excludes connections from other sites.
func TestPhysicalConnectionService_ListConnectionsBySite(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewPhysicalConnectionServiceClient(env.server.Client(), env.server.URL)

	// Site under test: one rack, two placed/ported assets, one connection.
	siteID := createSite(t, env, "Graph site")
	roomID := createRoom(t, env, siteID, "Graph room")
	rowID := createRackRow(t, env, roomID, "Graph row")
	rackID := createRack(t, env, rowID, "Graph rack", 42)

	catalogID := createCatalogEntry(t, env, "Graph model")
	aPort := createPortDefinition(t, env, catalogID, "eth0")
	bPort := createPortDefinition(t, env, catalogID, "eth1")
	aPlacement := placeAssetInRack(t, env, createAsset(t, env, catalogID), rackID, 1)
	bPlacement := placeAssetInRack(t, env, createAsset(t, env, catalogID), rackID, 2)

	createResp, err := client.CreatePhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePhysicalConnectionRequest_builder{
			SourcePlacementId:      aPlacement,
			SourcePortDefinitionId: aPort,
			TargetPlacementId:      bPlacement,
			TargetPortDefinitionId: bPort,
		}).Build(),
	))
	require.NoError(t, err)
	wantConnID := createResp.Msg.GetConnectionId()

	// A second site with its own connection that must not appear in the result.
	otherPlacementA, otherPortA, otherPlacementB, otherPortB := physicalConnectionFixture(t, env)
	_, err = client.CreatePhysicalConnection(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePhysicalConnectionRequest_builder{
			SourcePlacementId:      otherPlacementA,
			SourcePortDefinitionId: otherPortA,
			TargetPlacementId:      otherPlacementB,
			TargetPortDefinitionId: otherPortB,
		}).Build(),
	))
	require.NoError(t, err)

	resp, err := client.ListConnectionsBySite(context.Background(), connect.NewRequest(
		(&dcimv1.ListConnectionsBySiteRequest_builder{SiteId: siteID}).Build(),
	))
	require.NoError(t, err)

	got := make([]string, 0, len(resp.Msg.GetConnections()))
	for _, c := range resp.Msg.GetConnections() {
		got = append(got, c.GetId())
	}
	assert.Equal(t, []string{wantConnID}, got)
}
