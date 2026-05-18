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
	client := dcimv1connect.NewRackServiceClient(env.server.Client(), env.server.URL)

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
