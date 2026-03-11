package organization_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/fundament-oss/fundament/organization-api/pkg/clock"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_APIKey_Create_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	_, err := client.CreateAPIKey(context.Background(), connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
		Name:      "my-first-key",
		ExpiresIn: "",
	}.Build()))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_APIKey_Create(t *testing.T) {
	t.Parallel()

	testClock := clock.NewTest(time.Now().UTC())

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithClock(testClock),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewAPIKeyServiceClient(env.server.Client(), env.server.URL)

	inTwoMinutes := testClock.Now().Add(2 * time.Minute).Truncate(time.Microsecond)
	inFiveDays := testClock.Now().Add(120 * time.Hour).Truncate(time.Microsecond)

	tests := map[string]struct {
		CreateRequest *organizationv1.CreateAPIKeyRequest
		WantExpiresAt *time.Time
	}{
		"without_expiration": {
			CreateRequest: organizationv1.CreateAPIKeyRequest_builder{
				Name:      "my-first-key",
				ExpiresIn: "",
			}.Build(),
			WantExpiresAt: nil,
		},
		"with_expiration_in_minutes": {
			CreateRequest: organizationv1.CreateAPIKeyRequest_builder{
				Name:      "another-key",
				ExpiresIn: "2m",
			}.Build(),
			WantExpiresAt: &inTwoMinutes,
		},
		"with_expiration_in_hours": {
			CreateRequest: organizationv1.CreateAPIKeyRequest_builder{
				Name:      "yet-another-key",
				ExpiresIn: "120h",
			}.Build(),
			WantExpiresAt: &inFiveDays,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			createReq := connect.NewRequest(tc.CreateRequest)
			createReq.Header().Set("Authorization", "Bearer "+token)
			createReq.Header().Set("Fun-Organization", orgID.String())

			res, err := client.CreateAPIKey(context.Background(), createReq)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(res.Msg.GetToken(), "fun_"))
			assert.Equal(t, 40, len(res.Msg.GetToken()))
			assert.Equal(t, 8, len(res.Msg.GetTokenPrefix()))

			getReq := connect.NewRequest(organizationv1.GetAPIKeyRequest_builder{
				ApiKeyId: res.Msg.GetId(),
			}.Build())
			getReq.Header().Set("Authorization", "Bearer "+token)
			getReq.Header().Set("Fun-Organization", orgID.String())

			getRes, err := client.GetAPIKey(context.Background(), getReq)
			require.NoError(t, err)

			if tc.WantExpiresAt == nil {
				assert.False(t, getRes.Msg.GetApiKey().HasExpires())
			} else {
				require.True(t, getRes.Msg.GetApiKey().HasExpires())
				assert.Equal(t, *tc.WantExpiresAt, getRes.Msg.GetApiKey().GetExpires().AsTime())
			}
		})
	}
}

func Test_APIKey_Create_DuplicateName(t *testing.T) {
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

	createReq := func() *connect.Request[organizationv1.CreateAPIKeyRequest] {
		req := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
			Name: "duplicate-key",
		}.Build())
		req.Header().Set("Authorization", "Bearer "+token)
		req.Header().Set("Fun-Organization", orgID.String())
		return req
	}

	_, err := client.CreateAPIKey(context.Background(), createReq())
	require.NoError(t, err)

	_, err = client.CreateAPIKey(context.Background(), createReq())
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}
