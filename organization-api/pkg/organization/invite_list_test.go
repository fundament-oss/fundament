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

func Test_ListInvitations_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	_, err := client.ListInvitations(context.Background(), connect.NewRequest(&organizationv1.ListInvitationsRequest{}))

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}
func Test_ListInvitations_HappyFlow(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	userToInviteUUID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:    userToInviteUUID,
			Name:  "test-user2",
			Email: "foo@bar.baz",
		}),
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
	require.NotNil(t, invitationsRes.Msg.Invitations[0].Created.AsTime())
	require.Equal(t, "viewer", invitationsRes.Msg.Invitations[0].Permission)
	require.Equal(t, orgID.String(), invitationsRes.Msg.Invitations[0].OrganizationId)
	require.Equal(t, "test-org", invitationsRes.Msg.Invitations[0].OrganizationName)
}
