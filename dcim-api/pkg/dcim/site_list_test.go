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

func TestSiteService_ListSites_Populated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewSiteServiceClient(env.server.Client(), env.server.URL)

	want := []string{"List Site A", "List Site B", "List Site C"}
	for _, name := range want {
		createSite(t, env, name)
	}

	resp, err := client.ListSites(context.Background(), connect.NewRequest(&dcimv1.ListSitesRequest{}))
	require.NoError(t, err)

	got := make([]string, 0, len(resp.Msg.GetSites()))
	for _, s := range resp.Msg.GetSites() {
		got = append(got, s.GetName())
	}
	assert.ElementsMatch(t, want, got)
}
