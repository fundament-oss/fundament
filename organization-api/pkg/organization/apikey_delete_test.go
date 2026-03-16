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

	req := connect.NewRequest(organizationv1.DeleteAPIKeyRequest_builder{
		ApiKeyId: uuid.New().String(),
	}.Build())

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

	createReq := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "my-key",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(context.Background(), createReq)
	require.NoError(t, err)

	deleteReq := connect.NewRequest(organizationv1.DeleteAPIKeyRequest_builder{
		ApiKeyId: createRes.Msg.GetId(),
	}.Build())
	deleteReq.Header().Set("Authorization", "Bearer "+token)
	deleteReq.Header().Set("Fun-Organization", orgID.String())

	_, err = client.DeleteAPIKey(context.Background(), deleteReq)
	require.NoError(t, err)

	// Key should not appear in list after deletion.
	listReq := connect.NewRequest(organizationv1.ListAPIKeysRequest_builder{}.Build())
	listReq.Header().Set("Authorization", "Bearer "+token)
	listReq.Header().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListAPIKeys(context.Background(), listReq)
	require.NoError(t, err)
	assert.Empty(t, listRes.Msg.GetApiKeys())

	// Get should return NotFound after deletion.
	getReq := connect.NewRequest(organizationv1.GetAPIKeyRequest_builder{
		ApiKeyId: createRes.Msg.GetId(),
	}.Build())
	getReq.Header().Set("Authorization", "Bearer "+token)
	getReq.Header().Set("Fun-Organization", orgID.String())

	_, err = client.GetAPIKey(context.Background(), getReq)
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

	req := connect.NewRequest(organizationv1.DeleteAPIKeyRequest_builder{
		ApiKeyId: "00000000-0000-0000-0000-000000000000",
	}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	_, err := client.DeleteAPIKey(context.Background(), req)
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

	createReq := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "user-a-key",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+tokenA)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateAPIKey(context.Background(), createReq)
	require.NoError(t, err)

	deleteReq := connect.NewRequest(organizationv1.DeleteAPIKeyRequest_builder{
		ApiKeyId: createRes.Msg.GetId(),
	}.Build())
	deleteReq.Header().Set("Authorization", "Bearer "+tokenB)
	deleteReq.Header().Set("Fun-Organization", orgID.String())

	_, err = client.DeleteAPIKey(context.Background(), deleteReq)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
