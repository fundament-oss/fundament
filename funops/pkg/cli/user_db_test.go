package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The database keeps one live user per address (users_uq_email); these check
// that the commands turn its refusal into the intended error rather than the
// generic one, and that a second spelling of the address is the same address.

func TestUserCreate_RefusesSecondRegistrationAtSameAddress(t *testing.T) {
	ctx := newTestContext(t)

	require.NoError(t, (&UserCreateCmd{Email: "Someone@Example.com"}).Run(ctx))

	err := (&UserCreateCmd{Email: "someone@example.com"}).Run(ctx)
	require.EqualError(t, err, `user with email "someone@example.com" already exists`)

	assert.Equal(t, 1, liveUsersAt(t, ctx, "someone@example.com"))
}

func TestUserCreate_RefusesAddressOfSignedInUser(t *testing.T) {
	ctx := newTestContext(t)

	// Alice is in the test data with an identity provider account behind her.
	err := (&UserCreateCmd{Email: "ALICE@acme-corp.com"}).Run(ctx)
	require.EqualError(t, err, `user with email "ALICE@acme-corp.com" already exists`)

	assert.Equal(t, 1, liveUsersAt(t, ctx, "alice@acme-corp.com"))
}

func TestOrganizationMemberAdd_CreateUser_RefusesExistingAddress(t *testing.T) {
	ctx := newTestContext(t)

	first := &OrganizationMemberAddCmd{Organization: "acme-corp", User: "nobody@example.com", Permission: "viewer", CreateUser: true}
	require.NoError(t, first.Run(ctx))

	again := &OrganizationMemberAddCmd{Organization: "globex", User: "NOBODY@example.com", Permission: "viewer", CreateUser: true}
	err := again.Run(ctx)
	require.EqualError(t, err, `user with email "NOBODY@example.com" already exists: add them without --create-user`)

	assert.Equal(t, 1, liveUsersAt(t, ctx, "nobody@example.com"))

	// The refusal rolled the whole add back: no membership in globex either.
	members, err := ctx.Queries.MembershipList(t.Context(), membershipListParamsFor(t, ctx, "globex"))
	require.NoError(t, err)
	for _, m := range members {
		assert.NotEqual(t, "nobody@example.com", m.Email.String)
	}

	// Without the flag the existing registration is the one that gets added.
	without := &OrganizationMemberAddCmd{Organization: "globex", User: "nobody@example.com", Permission: "viewer"}
	require.NoError(t, without.Run(ctx))
}
