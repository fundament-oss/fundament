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

func Test_APIKey_List_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	_, err := client.ListAPIKeys(context.Background(), connect.NewRequest(organizationv1.ListAPIKeysRequest_builder{}.Build()))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_APIKey_List_Empty(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(organizationv1.ListAPIKeysRequest_builder{}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	res, err := client.ListAPIKeys(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, res.Msg.GetApiKeys())
}

func Test_APIKey_List(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	createReq := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "my-key",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(context.Background(), createReq)
	require.NoError(t, err)

	listReq := connect.NewRequest(organizationv1.ListAPIKeysRequest_builder{}.Build())
	listReq.Header().Set("Authorization", "Bearer "+token)
	listReq.Header().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListAPIKeys(context.Background(), listReq)
	require.NoError(t, err)

	require.Len(t, listRes.Msg.GetApiKeys(), 1)
	key := listRes.Msg.GetApiKeys()[0]
	assert.Equal(t, createRes.Msg.GetId(), key.GetId())
	assert.Equal(t, "my-key", key.GetName())
	assert.Equal(t, createRes.Msg.GetTokenPrefix(), key.GetTokenPrefix())
	assert.Equal(t, 8, len(key.GetTokenPrefix()))
}
