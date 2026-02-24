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

func Test_AcceptInvitation_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	_, err := client.AcceptInvitation(context.Background(), connect.NewRequest(&organizationv1.AcceptInvitationRequest{
		Id: "arbitrary",
	}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_AcceptInvitation_DoesNotExist(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(userID, "test-user", "", nil, []uuid.UUID{orgID}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(&organizationv1.AcceptInvitationRequest{
		Id: uuid.New().String(),
	})
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	_, err := client.AcceptInvitation(context.Background(), req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

func Test_AcceptInvitation_HappyFlow(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	userToInviteUUID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(userID, "test-user", "", nil, []uuid.UUID{orgID}),
		WithUser(userToInviteUUID, "test-user2", "foo@bar.baz", nil, []uuid.UUID{}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	req := connect.NewRequest(&organizationv1.InviteMemberRequest{
		Email:      "foo@bar.baz",
		Permission: "viewer",
	})
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("Fun-Organization", orgID.String())

	_, err := client.InviteMember(context.Background(), req)
	require.NoError(t, err)

	userToInviteToken := env.createAuthnToken(t, userToInviteUUID)

	listReq := connect.NewRequest(&organizationv1.ListInvitationsRequest{})
	listReq.Header().Set("Authorization", "Bearer "+userToInviteToken)
	listReq.Header().Set("Fun-Organization", orgID.String())

	invitationsRes, err := client.ListInvitations(context.Background(), listReq)
	require.NoError(t, err)

	require.NotNil(t, invitationsRes)
	require.Len(t, invitationsRes.Msg.Invitations, 1)

	acceptInvitationReq := connect.NewRequest(&organizationv1.AcceptInvitationRequest{
		Id: invitationsRes.Msg.Invitations[0].Id,
	})
	acceptInvitationReq.Header().Set("Authorization", "Bearer "+userToInviteToken)
	acceptInvitationReq.Header().Set("Fun-Organization", orgID.String())

	res, err := client.AcceptInvitation(context.Background(), acceptInvitationReq)
	require.NoError(t, err)
	require.NotNil(t, res)

	invitationsRes, err = client.ListInvitations(context.Background(), listReq)
	require.NoError(t, err)

	require.NotNil(t, invitationsRes)
	require.Len(t, invitationsRes.Msg.Invitations, 0)
}
