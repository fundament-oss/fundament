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

func TestRackService_ListRacks(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewRackServiceClient(env.server.Client(), env.server.URL)

	resp, err := client.ListRacks(context.Background(), connect.NewRequest(&dcimv1.ListRacksRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetRacks())
}
