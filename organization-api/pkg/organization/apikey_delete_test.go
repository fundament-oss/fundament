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

func Test_APIKey_Delete_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	req := organizationv1.DeleteAPIKeyRequest_builder{
		ApiKeyId: uuid.New().String(),
	}.Build()

	_, err := client.DeleteAPIKey(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_APIKey_Delete(t *testing.T) {
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

	deleteReq := organizationv1.DeleteAPIKeyRequest_builder{
		ApiKeyId: createRes.GetId(),
	}.Build()
	deleteCtx, deleteCallInfo := connect.NewClientContext(context.Background())
	deleteCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	deleteCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.DeleteAPIKey(deleteCtx, deleteReq)
	require.NoError(t, err)

	// Key should not appear in list after deletion.
	listReq := organizationv1.ListAPIKeysRequest_builder{}.Build()
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListAPIKeys(listCtx, listReq)
	require.NoError(t, err)
	assert.Empty(t, listRes.GetApiKeys())

	// Get should return NotFound after deletion.
	getReq := organizationv1.GetAPIKeyRequest_builder{
		ApiKeyId: createRes.GetId(),
	}.Build()
	getCtx, getCallInfo := connect.NewClientContext(context.Background())
	getCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	getCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.GetAPIKey(getCtx, getReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

func Test_APIKey_Delete_NotFound(t *testing.T) {
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

	req := organizationv1.DeleteAPIKeyRequest_builder{
		ApiKeyId: "00000000-0000-0000-0000-000000000000",
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err := client.DeleteAPIKey(ctx, req)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

func Test_APIKey_Delete_OtherUser_NotFound(t *testing.T) {
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

	deleteReq := organizationv1.DeleteAPIKeyRequest_builder{
		ApiKeyId: createRes.GetId(),
	}.Build()
	deleteCtx, deleteCallInfo := connect.NewClientContext(context.Background())
	deleteCallInfo.RequestHeader().Set("Authorization", "Bearer "+tokenB)
	deleteCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.DeleteAPIKey(deleteCtx, deleteReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
