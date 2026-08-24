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

func Test_APIKey_Revoke_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	req := organizationv1.RevokeAPIKeyRequest_builder{
		ApiKeyId: uuid.New().String(),
	}.Build()

	_, err := client.RevokeAPIKey(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_APIKey_Revoke(t *testing.T) {
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

	createReq := organizationv1.CreateAPIKeyRequest_builder{
		Name: "my-key",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(createCtx, createReq)
	require.NoError(t, err)

	revokeReq := organizationv1.RevokeAPIKeyRequest_builder{
		ApiKeyId: createRes.GetId(),
	}.Build()
	revokeCtx, revokeCallInfo := connect.NewClientContext(context.Background())
	revokeCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	revokeCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.RevokeAPIKey(revokeCtx, revokeReq)
	require.NoError(t, err)

	// Key should still be gettable with revoked timestamp set.
	getReq := organizationv1.GetAPIKeyRequest_builder{
		ApiKeyId: createRes.GetId(),
	}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	getRes, err := client.GetAPIKey(getCtx, getReq)
	require.NoError(t, err)
	assert.True(t, getRes.GetApiKey().HasRevoked())

	// Key should still appear in list.
	listReq := organizationv1.ListAPIKeysRequest_builder{}.Build()
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListAPIKeys(listCtx, listReq)
	require.NoError(t, err)
	assert.Len(t, listRes.GetApiKeys(), 1)
}

func Test_APIKey_Revoke_AlreadyRevoked(t *testing.T) {
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

	createReq := organizationv1.CreateAPIKeyRequest_builder{
		Name: "my-key",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(createCtx, createReq)
	require.NoError(t, err)

	revokeReq := func() *organizationv1.RevokeAPIKeyRequest {
		return organizationv1.RevokeAPIKeyRequest_builder{
			ApiKeyId: createRes.GetId(),
		}.Build()
	}

	revokeCtx, revokeCallInfo := connect.NewClientContext(context.Background())
	revokeCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	revokeCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.RevokeAPIKey(revokeCtx, revokeReq())
	require.NoError(t, err)

	_, err = client.RevokeAPIKey(revokeCtx, revokeReq())
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

func Test_APIKey_Revoke_OtherUser_NotFound(t *testing.T) {
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

	createReq := organizationv1.CreateAPIKeyRequest_builder{
		Name: "user-a-key",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+tokenA)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(createCtx, createReq)
	require.NoError(t, err)

	revokeReq := organizationv1.RevokeAPIKeyRequest_builder{
		ApiKeyId: createRes.GetId(),
	}.Build()
	revokeCtx, revokeCallInfo := connect.NewClientContext(context.Background())
	revokeCallInfo.RequestHeader().Set("Authorization", "Bearer "+tokenB)
	revokeCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.RevokeAPIKey(revokeCtx, revokeReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
