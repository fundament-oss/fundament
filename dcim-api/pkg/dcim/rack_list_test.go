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

func TestRackService_ListRacks_HappyFlow(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.client(), env.server.URL)

	rowID := createRackRowFixture(t, env, "Rack List")

	want := []string{"List Rack A", "List Rack B", "List Rack C"}
	for _, name := range want {
		createRack(t, env, rowID, name, 42)
	}

	resp, err := client.ListRacks(context.Background(), connect.NewRequest(
		(&dcimv1.ListRacksRequest_builder{RowId: &rowID}).Build(),
	))
	require.NoError(t, err)

	got := make([]string, 0, len(resp.Msg.GetRacks()))
	for _, summary := range resp.Msg.GetRacks() {
		got = append(got, summary.GetRack().GetName())
	}

	assert.ElementsMatch(t, want, got)
}

func TestRackService_ListRacks_FilterBySite(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.client(), env.server.URL)

	// Build a site with two rooms; each room has one rack row with one rack.
	siteID := createSite(t, env, "Target site")
	roomA := createRoom(t, env, siteID, "Room A")
	roomB := createRoom(t, env, siteID, "Room B")
	rowA := createRackRow(t, env, roomA, "Row A")
	rowB := createRackRow(t, env, roomB, "Row B")
	createRack(t, env, rowA, "Rack A", 42)
	createRack(t, env, rowB, "Rack B", 42)

	// A second site whose rack must NOT appear in the response.
	otherSite := createSite(t, env, "Other site")
	otherRoom := createRoom(t, env, otherSite, "Other room")
	otherRow := createRackRow(t, env, otherRoom, "Other row")
	createRack(t, env, otherRow, "Other rack", 42)

	resp, err := client.ListRacks(context.Background(), connect.NewRequest(
		(&dcimv1.ListRacksRequest_builder{SiteId: &siteID}).Build(),
	))
	require.NoError(t, err)

	got := make([]string, 0, len(resp.Msg.GetRacks()))
	for _, summary := range resp.Msg.GetRacks() {
		got = append(got, summary.GetRack().GetName())
	}

	assert.ElementsMatch(t, []string{"Rack A", "Rack B"}, got)
}
