package organization_test

import (
	"context"
	"fmt"
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
		WithUser(userID, "test-user", "", nil, []uuid.UUID{orgID}),
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
	assert.Equal(t, "pending", res.Msg.Member.Status)
}

func Test_InviteMember_ExistingUser(t *testing.T) {
	t.Parallel()

	orgAID := uuid.New()
	orgBID := uuid.New()
	userID := uuid.New()
	userID2 := uuid.New()

	externalRef := fmt.Sprintf("ext_%s", userID2.String())

	env := newTestAPI(t,
		WithOrganization(orgAID, "test-org-a"),
		WithOrganization(orgBID, "test-org-b"),
		WithUser(userID, "test-user", "", nil, []uuid.UUID{orgAID}),
		WithUser(userID2, "second-user", "foo@bar.baz", &externalRef, []uuid.UUID{}),
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

	require.NotNil(t, res.Msg.Member)
	assert.Equal(t, "viewer", res.Msg.Member.Permission)
	assert.Equal(t, "foo@bar.baz", *res.Msg.Member.Email)
	assert.Equal(t, &externalRef, res.Msg.Member.ExternalRef)
	assert.Equal(t, "second-user", res.Msg.Member.Name)
	assert.Equal(t, "pending", res.Msg.Member.Status)
}

func Test_InviteMember_ExistingUser_AlreadyMember(t *testing.T) {
	t.Parallel()

	orgAID := uuid.New()
	orgBID := uuid.New()
	userID := uuid.New()
	userID2 := uuid.New()

	externalRef := fmt.Sprintf("ext_%s", userID2.String())

	env := newTestAPI(t,
		WithOrganization(orgAID, "test-org-a"),
		WithOrganization(orgBID, "test-org-b"),
		WithUser(userID, "test-user", "", nil, []uuid.UUID{orgAID}),
		WithUser(userID2, "second-user", "foo@bar.baz", &externalRef, []uuid.UUID{}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(&organizationv1.InviteMemberRequest{
		Email:      "foo@bar.baz",
		Permission: "viewer",
	})
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgAID.String())

	_, err := client.InviteMember(context.Background(), req)
	require.NoError(t, err)

	_, err = client.InviteMember(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}
