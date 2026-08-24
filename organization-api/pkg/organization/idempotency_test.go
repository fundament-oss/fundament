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

	newReq := func() *organizationv1.CreateAPIKeyRequest {
		return organizationv1.CreateAPIKeyRequest_builder{
			Name: "idempotent-key",
		}.Build()
	}
	newCtx := func() (context.Context, connect.CallInfo) {
		ctx, callInfo := connect.NewClientContext(context.Background())
		callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
		callInfo.RequestHeader().Set("Fun-Organization", orgID.String())
		callInfo.RequestHeader().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)
		return ctx, callInfo
	}

	// First call: creates the API key.
	ctx1, callInfo1 := newCtx()
	res1, err := client.CreateAPIKey(ctx1, newReq())
	require.NoError(t, err)
	assert.NotEmpty(t, res1.GetId())
	assert.Equal(t, "processing", callInfo1.ResponseHeader().Get(idempotency.HeaderIdempotencyStatus))

	// Replay: same idempotency key returns the cached response.
	ctx2, callInfo2 := newCtx()
	res2, err := client.CreateAPIKey(ctx2, newReq())
	require.NoError(t, err)
	assert.Equal(t, res1.GetId(), res2.GetId())
	assert.NotEmpty(t, callInfo2.ResponseHeader().Get(idempotency.HeaderIdempotencyStatus))
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
	req1 := organizationv1.CreateAPIKeyRequest_builder{
		Name: "key-one",
	}.Build()
	ctx1, callInfo1 := connect.NewClientContext(context.Background())
	callInfo1.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo1.RequestHeader().Set("Fun-Organization", orgID.String())
	callInfo1.RequestHeader().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)

	_, err := client.CreateAPIKey(ctx1, req1)
	require.NoError(t, err)

	// Replay with different request body should fail.
	req2 := organizationv1.CreateAPIKeyRequest_builder{
		Name: "key-two",
	}.Build()
	ctx2, callInfo2 := connect.NewClientContext(context.Background())
	callInfo2.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo2.RequestHeader().Set("Fun-Organization", orgID.String())
	callInfo2.RequestHeader().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)

	_, err = client.CreateAPIKey(ctx2, req2)
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
	req := organizationv1.CreateAPIKeyRequest_builder{
		Name: "no-idempotency",
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.CreateAPIKey(ctx, req)
	require.NoError(t, err)
	assert.NotEmpty(t, res.GetId())
	assert.Empty(t, callInfo.ResponseHeader().Get(idempotency.HeaderIdempotencyStatus))
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
	req1 := organizationv1.CreateAPIKeyRequest_builder{
		Name: "user1-key",
	}.Build()
	ctx1, callInfo1 := connect.NewClientContext(context.Background())
	callInfo1.RequestHeader().Set("Authorization", "Bearer "+env.createAuthnToken(t, user1ID))
	callInfo1.RequestHeader().Set("Fun-Organization", orgID.String())
	callInfo1.RequestHeader().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)

	res1, err := client.CreateAPIKey(ctx1, req1)
	require.NoError(t, err)

	// User 2 uses the same idempotency key — should create a separate resource.
	req2 := organizationv1.CreateAPIKeyRequest_builder{
		Name: "user2-key",
	}.Build()
	ctx2, callInfo2 := connect.NewClientContext(context.Background())
	callInfo2.RequestHeader().Set("Authorization", "Bearer "+env.createAuthnToken(t, user2ID))
	callInfo2.RequestHeader().Set("Fun-Organization", orgID.String())
	callInfo2.RequestHeader().Set(idempotency.HeaderIdempotencyKey, idempotencyKey)

	res2, err := client.CreateAPIKey(ctx2, req2)
	require.NoError(t, err)

	assert.NotEqual(t, res1.GetId(), res2.GetId())
}
