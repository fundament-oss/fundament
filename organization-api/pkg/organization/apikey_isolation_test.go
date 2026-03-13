package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_APIKey_Isolation_List(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userAID := uuid.New()
	userBID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userAID,
			Name:   "user-a",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     userBID,
			Name:   "user-b",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	tokenA := env.createAuthnToken(t, userAID)
	tokenB := env.createAuthnToken(t, userBID)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	createKey := func(token, name string) {
		req := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
			Name: name,
		}.Build())
		req.Header().Set("Authorization", "Bearer "+token)
		req.Header().Set("Fun-Organization", orgID.String())
		_, err := client.CreateAPIKey(context.Background(), req)
		require.NoError(t, err)
	}

	listKeys := func(token string) []*organizationv1.APIKey {
		req := connect.NewRequest(organizationv1.ListAPIKeysRequest_builder{}.Build())
		req.Header().Set("Authorization", "Bearer "+token)
		req.Header().Set("Fun-Organization", orgID.String())
		res, err := client.ListAPIKeys(context.Background(), req)
		require.NoError(t, err)
		return res.Msg.GetApiKeys()
	}

	createKey(tokenA, "key-a-1")
	createKey(tokenA, "key-a-2")
	createKey(tokenB, "key-b-1")

	keysA := listKeys(tokenA)
	keysB := listKeys(tokenB)

	assert.Len(t, keysA, 2, "user A should see exactly 2 keys")
	assert.Len(t, keysB, 1, "user B should see exactly 1 key")

	namesA := make([]string, 0, len(keysA))
	for _, k := range keysA {
		namesA = append(namesA, k.GetName())
	}
	assert.ElementsMatch(t, []string{"key-a-1", "key-a-2"}, namesA)

	assert.Equal(t, "key-b-1", keysB[0].GetName())
}
