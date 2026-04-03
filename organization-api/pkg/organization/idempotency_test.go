package organization_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/idempotency"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

func Test_Idempotency_CreateAPIKey_Replay(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			Email:  "test@example.com",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithIdempotency(),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)
	idempotencyKey := uuid.New().String()

	newReq := func() *connect.Request[organizationv1.CreateAPIKeyRequest] {
		req := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
			Name: "idempotent-key",
		}.Build())
		req.Header().Set("Authorization", "Bearer "+token)
		req.Header().Set("Fun-Organization", orgID.String())
		req.Header().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)
		return req
	}

	// First call: creates the API key.
	res1, err := client.CreateAPIKey(context.Background(), newReq())
	require.NoError(t, err)
	assert.NotEmpty(t, res1.Msg.GetId())
	assert.Equal(t, "processing", res1.Header().Get(idempotency.HeaderIdempotencyStatus))

	// Replay: same idempotency key returns the cached response.
	res2, err := client.CreateAPIKey(context.Background(), newReq())
	require.NoError(t, err)
	assert.Equal(t, res1.Msg.GetId(), res2.Msg.GetId())
	assert.NotEmpty(t, res2.Header().Get(idempotency.HeaderIdempotencyStatus))
}

func Test_Idempotency_DifferentRequestBody_Rejected(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			Email:  "test2@example.com",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithIdempotency(),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)
	idempotencyKey := uuid.New().String()

	// First call.
	req1 := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "key-one",
	}.Build())
	req1.Header().Set("Authorization", "Bearer "+token)
	req1.Header().Set("Fun-Organization", orgID.String())
	req1.Header().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)

	_, err := client.CreateAPIKey(context.Background(), req1)
	require.NoError(t, err)

	// Replay with different request body should fail.
	req2 := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "key-two",
	}.Build())
	req2.Header().Set("Authorization", "Bearer "+token)
	req2.Header().Set("Fun-Organization", orgID.String())
	req2.Header().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)

	_, err = client.CreateAPIKey(context.Background(), req2)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func Test_Idempotency_NoHeader_Passthrough(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			Email:  "test3@example.com",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithIdempotency(),
	)

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	// Request without Idempotency-Key header should pass through normally.
	req := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "no-idempotency",
	}.Build())
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	res, err := client.CreateAPIKey(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Msg.GetId())
	assert.Empty(t, res.Header().Get(idempotency.HeaderIdempotencyStatus))
}

func Test_Idempotency_DifferentUsers_SameKey(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	user1ID := uuid.New()
	user2ID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     user1ID,
			Name:   "user-one",
			Email:  "user1@example.com",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     user2ID,
			Name:   "user-two",
			Email:  "user2@example.com",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithIdempotency(),
	)

	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)
	idempotencyKey := uuid.New().String()

	// User 1 creates with the key.
	req1 := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "user1-key",
	}.Build())
	req1.Header().Set("Authorization", "Bearer "+env.createAuthnToken(t, user1ID))
	req1.Header().Set("Fun-Organization", orgID.String())
	req1.Header().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)

	res1, err := client.CreateAPIKey(context.Background(), req1)
	require.NoError(t, err)

	// User 2 uses the same idempotency key — should create a separate resource.
	req2 := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name: "user2-key",
	}.Build())
	req2.Header().Set("Authorization", "Bearer "+env.createAuthnToken(t, user2ID))
	req2.Header().Set("Fun-Organization", orgID.String())
	req2.Header().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)

	res2, err := client.CreateAPIKey(context.Background(), req2)
	require.NoError(t, err)

	assert.NotEqual(t, res1.Msg.GetId(), res2.Msg.GetId())
}
