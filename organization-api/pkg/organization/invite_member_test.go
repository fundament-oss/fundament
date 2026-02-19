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

func Test_InviteMember_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	_, err := client.InviteMember(context.Background(), connect.NewRequest(&organizationv1.InviteMemberRequest{
		Email:      "arbitrary",
		Permission: "arbitrary",
	}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_InviteMember_NewUser(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(userID, "test-user", "", []uuid.UUID{orgID}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(&organizationv1.InviteMemberRequest{
		Email:      "foo@bar.baz",
		Permission: "viewer",
	})
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	res, err := client.InviteMember(context.Background(), req)
	require.NoError(t, err)

	require.NotNil(t, res.Msg.Member)
	assert.Equal(t, "viewer", res.Msg.Member.Permission)
	assert.Equal(t, "foo@bar.baz", *res.Msg.Member.Email)
	assert.Nil(t, res.Msg.Member.ExternalRef)
	assert.Equal(t, "", res.Msg.Member.Name)
	assert.Equal(t, "", res.Msg.Member.Status)
}

func Test_InviteMember_ExistingUser(t *testing.T) {
	t.Parallel()

	orgAID := uuid.New()
	orgBID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgAID, "test-org-a"),
		WithOrganization(orgBID, "test-org-b"),
		WithUser(userID, "test-user", "", []uuid.UUID{orgAID}),
		WithUser(userID, "second-user", "foo@bar.baz", []uuid.UUID{orgBID}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(&organizationv1.InviteMemberRequest{
		Email:      "foo@bar.baz",
		Permission: "viewer",
	})
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgAID.String())

	res, err := client.InviteMember(context.Background(), req)
	require.NoError(t, err)

	// I expect userB to be added, instead the test fails with
	// 'permission_denied: user is not a member of organization 7fdf4b0a-113a-49dc-bacf-4a740081a320' (=orgA)
	require.NotNil(t, res.Msg.Member)
	assert.Equal(t, "viewer", res.Msg.Member.Permission)
	assert.Equal(t, "foo@bar.baz", *res.Msg.Member.Email)
	assert.Nil(t, res.Msg.Member.ExternalRef)
	assert.Equal(t, "", res.Msg.Member.Name)
	assert.Equal(t, "", res.Msg.Member.Status)
}
