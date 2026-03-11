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

	req := connect.NewRequest(organizationv1.RevokeAPIKeyRequest_builder{
		ApiKeyId: uuid.New().String(),
	}.Build())

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

	createReq := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "my-key",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(context.Background(), createReq)
	require.NoError(t, err)

	revokeReq := connect.NewRequest(organizationv1.RevokeAPIKeyRequest_builder{
		ApiKeyId: createRes.Msg.GetId(),
	}.Build())
	revokeReq.Header().Set("Authorization", "Bearer "+token)
	revokeReq.Header().Set("Fun-Organization", orgID.String())

	_, err = client.RevokeAPIKey(context.Background(), revokeReq)
	require.NoError(t, err)

	// Key should still be gettable with revoked timestamp set.
	getReq := connect.NewRequest(organizationv1.GetAPIKeyRequest_builder{
		ApiKeyId: createRes.Msg.GetId(),
	}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())

	getRes, err := client.GetAPIKey(context.Background(), getReq)
	require.NoError(t, err)
	assert.True(t, getRes.Msg.GetApiKey().HasRevoked())

	// Key should still appear in list.
	listReq := connect.NewRequest(organizationv1.ListAPIKeysRequest_builder{}.Build())
	listReq.Header().Set("Authorization", "Bearer "+token)
	listReq.Header().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListAPIKeys(context.Background(), listReq)
	require.NoError(t, err)
	assert.Len(t, listRes.Msg.GetApiKeys(), 1)
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

	createReq := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "my-key",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(context.Background(), createReq)
	require.NoError(t, err)

	revokeReq := func() *connect.Request[organizationv1.RevokeAPIKeyRequest] {
		req := connect.NewRequest(organizationv1.RevokeAPIKeyRequest_builder{
			ApiKeyId: createRes.Msg.GetId(),
		}.Build())
		req.Header().Set("Authorization", "Bearer "+token)
		req.Header().Set("Fun-Organization", orgID.String())
		return req
	}

	_, err = client.RevokeAPIKey(context.Background(), revokeReq())
	require.NoError(t, err)

	_, err = client.RevokeAPIKey(context.Background(), revokeReq())
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

	createReq := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "user-a-key",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+tokenA)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(context.Background(), createReq)
	require.NoError(t, err)

	revokeReq := connect.NewRequest(organizationv1.RevokeAPIKeyRequest_builder{
		ApiKeyId: createRes.Msg.GetId(),
	}.Build())
	revokeReq.Header().Set("Authorization", "Bearer "+tokenB)
	revokeReq.Header().Set("Fun-Organization", orgID.String())

	_, err = client.RevokeAPIKey(context.Background(), revokeReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
