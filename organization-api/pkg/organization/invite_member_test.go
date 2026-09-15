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
	"google.golang.org/protobuf/proto"
)

func Test_InviteMember_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	_, err := client.InviteMember(context.Background(), organizationv1.InviteMemberRequest_builder{
		Email:      "arbitrary",
		Permission: "arbitrary",
	}.Build())

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
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
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

	res, err := client.InviteMember(ctx, req)
	require.NoError(t, err)

	require.NotNil(t, res)
	assert.NotEqual(t, "", res.GetInvitationId())
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
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgAID},
		}),
		WithUser(&UserArgs{
			ID:          userID2,
			Name:        "second-user",
			Email:       "foo@bar.baz",
			ExternalRef: &externalRef,
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
	callInfo.RequestHeader().Set("Fun-Organization", orgAID.String())

	res, err := client.InviteMember(ctx, req)
	require.NoError(t, err)

	require.NotNil(t, res.GetInvitationId())
	assert.NotEqual(t, "", res.GetInvitationId())
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
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgAID},
		}),
		WithUser(&UserArgs{
			ID:          userID2,
			Name:        "second-user",
			Email:       "foo@bar.baz",
			ExternalRef: &externalRef,
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
	callInfo.RequestHeader().Set("Fun-Organization", orgAID.String())

	_, err := client.InviteMember(ctx, req)
	require.NoError(t, err)

	_, err = client.InviteMember(ctx, req)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}

// Test_InviteMember_ExistingUser_DifferentlyCapitalisedEmail: an address is the
// same address however it was typed. Inviting someone at a spelling that only
// differs in capitalisation has to reach the account they already have, rather
// than leave a second account behind that they can never sign in to — signing
// in would miss it and start a new organization instead of joining this one.
func Test_InviteMember_ExistingUser_DifferentlyCapitalisedEmail(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	inviteeID := uuid.New()

	externalRef := fmt.Sprintf("ext_%s", inviteeID.String())

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:          inviteeID,
			Name:        "second-user",
			Email:       "foo@bar.baz",
			ExternalRef: &externalRef,
		}),
	)

	token := env.createAuthnToken(t, userID)

	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	inviteClient := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	res, err := inviteClient.InviteMember(ctx, organizationv1.InviteMemberRequest_builder{
		Email:      "FOO@Bar.Baz",
		Permission: "viewer",
	}.Build())
	require.NoError(t, err)

	memberClient := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	member, err := memberClient.GetMember(ctx, organizationv1.GetMemberRequest_builder{
		Id: proto.String(res.GetInvitationId()),
	}.Build())
	require.NoError(t, err)

	assert.Equal(t, inviteeID.String(), member.GetMember().GetUserId())
}

// Test_InviteMember_AlreadyMember_DifferentlyCapitalisedEmail: the same goes for
// someone already in the organization — a differently typed spelling of their
// address is still their address, so the invitation is refused as a duplicate
// rather than quietly making a second member out of the same person.
func Test_InviteMember_AlreadyMember_DifferentlyCapitalisedEmail(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()
	memberID := uuid.New()

	externalRef := fmt.Sprintf("ext_%s", memberID.String())

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:          memberID,
			Name:        "second-user",
			Email:       "foo@bar.baz",
			ExternalRef: &externalRef,
			OrgIDs:      []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	client := organizationv1connect.NewInviteServiceClient(env.server.Client(), env.server.URL)

	_, err := client.InviteMember(ctx, organizationv1.InviteMemberRequest_builder{
		Email:      "FOO@Bar.Baz",
		Permission: "viewer",
	}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}
