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

func TestRackRowService_ListRackRows_FilterBySite(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackRowServiceClient(env.server.Client(), env.server.URL)

	// Two rooms in the target site, one rack row each.
	siteID := createSite(t, env, "Target site")
	roomA := createRoom(t, env, siteID, "Room A")
	roomB := createRoom(t, env, siteID, "Room B")
	createRackRow(t, env, roomA, "Row A")
	createRackRow(t, env, roomB, "Row B")

	// A second site's row must not appear.
	otherSite := createSite(t, env, "Other site")
	otherRoom := createRoom(t, env, otherSite, "Other room")
	createRackRow(t, env, otherRoom, "Other row")

	resp, err := client.ListRackRows(context.Background(), connect.NewRequest(
		(&dcimv1.ListRackRowsRequest_builder{SiteId: &siteID}).Build(),
	))
	require.NoError(t, err)

	got := make([]string, 0, len(resp.Msg.GetRackRows()))
	for _, row := range resp.Msg.GetRackRows() {
		got = append(got, row.GetName())
	}

	assert.ElementsMatch(t, []string{"Row A", "Row B"}, got)
}
