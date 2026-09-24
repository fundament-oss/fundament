package cli

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// Users from db/testdata: Alice and Bart are in acme-corp, David in globex.
var (
	testUserAlice = uuid.MustParse("019b4000-1000-7000-8000-000000000001")
	testUserBart  = uuid.MustParse("019b4000-1000-7000-8000-000000000002")
)

// inviteToGlobex writes an invitation row into globex in the given status, the
// way the organization-api's invite flow would (pending) or the person would
// leave it (declined). Globex, because the acme-corp users these tests invite
// are not in it.
func inviteToGlobex(t *testing.T, ctx *Context, userID uuid.UUID, permission, status string) {
	t.Helper()

	orgID, err := lookupOrganizationID(t.Context(), ctx.Queries, "globex")
	require.NoError(t, err)
	_, err = ctx.DB.Pool.Exec(t.Context(),
		`INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status) VALUES ($1, $2, $3, $4)`,
		orgID, userID, permission, status)
	require.NoError(t, err)
}

// membershipOf finds a user's live row in an organization's member list.
func membershipOf(t *testing.T, ctx *Context, organization string, userID uuid.UUID) *db.MembershipListRow {
	t.Helper()

	members, err := ctx.Queries.MembershipList(t.Context(), membershipListParamsFor(t, ctx, organization))
	require.NoError(t, err)
	for i := range members {
		if members[i].UserID == userID {
			return &members[i]
		}
	}
	return nil
}

func TestOrganizationMemberAdd_AcceptsPendingInvitationKeepingItsPermission(t *testing.T) {
	ctx := newTestContext(t)
	inviteToGlobex(t, ctx, testUserBart, "admin", "pending")

	// No --permission: the invitation was sent as admin, and stays admin.
	cmd := &OrganizationMemberAddCmd{Organization: "globex", User: testUserBart.String()}
	require.NoError(t, cmd.Run(ctx))

	m := membershipOf(t, ctx, "globex", testUserBart)
	require.NotNil(t, m)
	assert.Equal(t, dbconst.OrganizationsUserStatus_Accepted, m.Status)
	assert.Equal(t, dbconst.OrganizationsUserPermission_Admin, m.Permission)
}

func TestOrganizationMemberAdd_AcceptsPendingInvitationWithGivenPermission(t *testing.T) {
	ctx := newTestContext(t)
	inviteToGlobex(t, ctx, testUserBart, "admin", "pending")

	cmd := &OrganizationMemberAddCmd{Organization: "globex", User: testUserBart.String(), Permission: "viewer"}
	require.NoError(t, cmd.Run(ctx))

	m := membershipOf(t, ctx, "globex", testUserBart)
	require.NotNil(t, m)
	assert.Equal(t, dbconst.OrganizationsUserStatus_Accepted, m.Status)
	assert.Equal(t, dbconst.OrganizationsUserPermission_Viewer, m.Permission)
}

func TestOrganizationMemberAdd_RefusesAnExistingMember(t *testing.T) {
	ctx := newTestContext(t)

	cmd := &OrganizationMemberAddCmd{Organization: "acme-corp", User: "alice@acme-corp.com"}
	require.EqualError(t, cmd.Run(ctx), `user "alice@acme-corp.com" is already a member of organization "acme-corp"`)

	// Untouched: still the admin she was seeded as.
	m := membershipOf(t, ctx, "acme-corp", testUserAlice)
	require.NotNil(t, m)
	assert.Equal(t, dbconst.OrganizationsUserPermission_Admin, m.Permission)
}

func TestOrganizationMemberRemove_DeclinedInvitationIsNothingToRemove(t *testing.T) {
	ctx := newTestContext(t)
	inviteToGlobex(t, ctx, testUserBart, "viewer", "declined")

	cmd := &OrganizationMemberRemoveCmd{Organization: "globex", User: testUserBart.String()}
	require.EqualError(t, cmd.Run(ctx), `user "`+testUserBart.String()+`" is not a member of organization "globex"`)
}

func TestOrganizationMemberRemove_RevokesPendingInvitation(t *testing.T) {
	ctx := newTestContext(t)
	inviteToGlobex(t, ctx, testUserBart, "viewer", "pending")

	cmd := &OrganizationMemberRemoveCmd{Organization: "globex", User: testUserBart.String()}
	require.NoError(t, cmd.Run(ctx))

	assert.Nil(t, membershipOf(t, ctx, "globex", testUserBart))
}

func TestUserList_InvitationIsNotMembership(t *testing.T) {
	ctx := newTestContext(t)
	require.NoError(t, (&UserCreateCmd{Email: "invited@example.com"}).Run(ctx))
	users, err := ctx.Queries.UserList(t.Context())
	require.NoError(t, err)
	var invited *db.UserListRow
	for i := range users {
		if users[i].Email.String == "invited@example.com" {
			invited = &users[i]
		}
	}
	require.NotNil(t, invited)
	inviteToGlobex(t, ctx, invited.ID, "viewer", "pending")

	users, err = ctx.Queries.UserList(t.Context())
	require.NoError(t, err)
	for i := range users {
		if users[i].ID == invited.ID {
			assert.Empty(t, users[i].OrganizationNames, "an unanswered invitation is not an organization they are in")
			assert.Equal(t, []string{"globex"}, users[i].InvitationNames)
		}
	}
}

func TestUserDelete_RemovesARegistrationAndItsMemberships(t *testing.T) {
	ctx := newTestContext(t)
	add := &OrganizationMemberAddCmd{Organization: "globex", User: "typo@example.com", CreateUser: true}
	require.NoError(t, add.Run(ctx))
	require.Equal(t, 1, liveUsersAt(t, ctx, "typo@example.com"))

	require.NoError(t, (&UserDeleteCmd{User: "typo@example.com"}).Run(ctx))

	assert.Equal(t, 0, liveUsersAt(t, ctx, "typo@example.com"))
	members, err := ctx.Queries.MembershipList(t.Context(), membershipListParamsFor(t, ctx, "globex"))
	require.NoError(t, err)
	for _, m := range members {
		assert.NotEqual(t, "typo@example.com", m.Email.String)
	}
	// And the address is free again.
	require.NoError(t, (&UserCreateCmd{Email: "typo@example.com"}).Run(ctx))
}

func TestUserDelete_RefusesAnAccountSomebodySignedInTo(t *testing.T) {
	ctx := newTestContext(t)

	err := (&UserDeleteCmd{User: "alice@acme-corp.com"}).Run(ctx)
	require.EqualError(t, err, `user "alice@acme-corp.com" has signed in and is not a registration; only registrations nobody has signed in with can be deleted`)
	assert.Equal(t, 1, liveUsersAt(t, ctx, "alice@acme-corp.com"))
}
