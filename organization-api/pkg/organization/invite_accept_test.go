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

	_, err := client.AcceptInvitation(context.Background(), organizationv1.AcceptInvitationRequest_builder{
		Id: "arbitrary",
	}.Build())

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
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	req := organizationv1.AcceptInvitationRequest_builder{
		Id: uuid.New().String(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err := client.AcceptInvitation(ctx, req)

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

	req := organizationv1.InviteMemberRequest_builder{
		Email:      "foo@bar.baz",
		Permission: "viewer",
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err := client.InviteMember(ctx, req)
	require.NoError(t, err)

	userToInviteToken := env.createAuthnToken(t, userToInviteUUID)

	listReq := &organizationv1.ListInvitationsRequest{}
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+userToInviteToken)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	invitationsRes, err := client.ListInvitations(listCtx, listReq)
	require.NoError(t, err)

	require.NotNil(t, invitationsRes)
	require.Len(t, invitationsRes.GetInvitations(), 1)

	acceptInvitationReq := organizationv1.AcceptInvitationRequest_builder{
		Id: invitationsRes.GetInvitations()[0].GetId(),
	}.Build()
	acceptInvitationCtx, acceptInvitationCallInfo := connect.NewClientContext(context.Background())
	acceptInvitationCallInfo.RequestHeader().Set("Authorization", "Bearer "+userToInviteToken)
	acceptInvitationCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.AcceptInvitation(acceptInvitationCtx, acceptInvitationReq)
	require.NoError(t, err)
	require.NotNil(t, res)

	invitationsRes, err = client.ListInvitations(listCtx, listReq)
	require.NoError(t, err)

	require.NotNil(t, invitationsRes)
	require.Len(t, invitationsRes.GetInvitations(), 0)
}
