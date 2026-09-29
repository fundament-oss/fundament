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

func Test_Member_List_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	_, err := client.ListMembers(context.Background(), &organizationv1.ListMembersRequest{})

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Member_List(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "my-organization"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     uuid.New(),
			Name:   "user2",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     uuid.New(),
			Name:   "user3",
			OrgIDs: []uuid.UUID{orgID},
		}),
		WithUser(&UserArgs{
			ID:     uuid.New(),
			Name:   "user4",
			OrgIDs: []uuid.UUID{},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	listReq := &organizationv1.ListMembersRequest{}
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	listRes, err := client.ListMembers(listCtx, listReq)
	require.NoError(t, err)

	require.Len(t, listRes.GetMembers(), 3)
}

// A user who belongs to two organizations must only appear once in each
// organization's member list. The memberships table lets a user select their
// own rows across organizations (the invitation flow needs that), so the list
// query has to pin the organization itself.
func Test_Member_List_UserInTwoOrganizations(t *testing.T) {
	t.Parallel()

	orgAID := uuid.New()
	orgBID := uuid.New()
	callerUserID := uuid.New()
	orgAOnlyUserID := uuid.New()
	orgBOnlyUserID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgAID, "org-a"),
		WithOrganization(orgBID, "org-b"),
		WithUser(&UserArgs{
			ID:     callerUserID,
			Name:   "caller-user",
			OrgIDs: []uuid.UUID{orgAID, orgBID},
		}),
		WithUser(&UserArgs{
			ID:     orgAOnlyUserID,
			Name:   "org-a-user",
			OrgIDs: []uuid.UUID{orgAID},
		}),
		WithUser(&UserArgs{
			ID:     orgBOnlyUserID,
			Name:   "org-b-user",
			OrgIDs: []uuid.UUID{orgBID},
		}),
	)

	token := env.createAuthnToken(t, callerUserID)

	client := organizationv1connect.NewMemberServiceClient(env.server.Client(), env.server.URL)

	listMembers := func(t *testing.T, orgID uuid.UUID) []*organizationv1.Member {
		t.Helper()

		ctx, callInfo := connect.NewClientContext(context.Background())
		callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
		callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

		res, err := client.ListMembers(ctx, &organizationv1.ListMembersRequest{})
		require.NoError(t, err)

		return res.GetMembers()
	}

	userIDs := func(members []*organizationv1.Member) []string {
		ids := make([]string, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.GetUserId())
		}
		return ids
	}

	membersA := listMembers(t, orgAID)
	assert.ElementsMatch(t, []string{callerUserID.String(), orgAOnlyUserID.String()}, userIDs(membersA))

	membersB := listMembers(t, orgBID)
	assert.ElementsMatch(t, []string{callerUserID.String(), orgBOnlyUserID.String()}, userIDs(membersB))
}
