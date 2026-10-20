package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/fundament-oss/fundament/dcim-authn-api/pkg/db/gen"
)

// These tests run UserUpsert as fun_dcim_authn_api against a real database:
// the query holds the identity rules (patch semantics, the partial conflict
// target) and the grants must admit exactly the writes it needs.

func upsert(t *testing.T, env *testEnv, name, ref, email string) db.UserUpsertRow {
	t.Helper()
	row, err := env.queries.UserUpsert(context.Background(), db.UserUpsertParams{
		Name:        name,
		ExternalRef: ref,
		Email:       email,
	})
	require.NoError(t, err)
	return row
}

func userByID(t *testing.T, env *testEnv, id any) (name string, email, deleted *string) {
	t.Helper()
	err := env.superPool.QueryRow(context.Background(),
		`SELECT name, email, deleted::text FROM dcim.users WHERE id = $1`, id).
		Scan(&name, &email, &deleted)
	require.NoError(t, err)
	return name, email, deleted
}

// A first login records what the IdP sent and returns the fresh internal id —
// the value that becomes the DCIM JWT subject.
func TestUserUpsert_FirstLoginProvisions(t *testing.T) {
	env := newTestEnv(t)

	row := upsert(t, env, "Alice", "idp-sub-1", "alice@example.com")

	assert.Equal(t, "Alice", row.Name)
	name, email, deleted := userByID(t, env, row.ID)
	assert.Equal(t, "Alice", name)
	require.NotNil(t, email)
	assert.Equal(t, "alice@example.com", *email)
	assert.Nil(t, deleted)
}

// The update is a patch, not a replace: a connector that omits the name or
// email claim must not erase what the directory already holds.
func TestUserUpsert_AbsentClaimsKeepDirectoryValues(t *testing.T) {
	env := newTestEnv(t)
	first := upsert(t, env, "Alice", "idp-sub-1", "alice@example.com")

	patched := upsert(t, env, "", "idp-sub-1", "")

	assert.Equal(t, first.ID, patched.ID)
	assert.Equal(t, "Alice", patched.Name)
	name, email, _ := userByID(t, env, first.ID)
	assert.Equal(t, "Alice", name)
	require.NotNil(t, email)
	assert.Equal(t, "alice@example.com", *email)
}

// Present claims do land: a rename at the IdP shows up on the next login.
func TestUserUpsert_PresentClaimsUpdate(t *testing.T) {
	env := newTestEnv(t)
	first := upsert(t, env, "Alice", "idp-sub-1", "alice@example.com")

	renamed := upsert(t, env, "Alice Renamed", "idp-sub-1", "")

	assert.Equal(t, first.ID, renamed.ID)
	assert.Equal(t, "Alice Renamed", renamed.Name)
	_, email, _ := userByID(t, env, first.ID)
	require.NotNil(t, email)
	assert.Equal(t, "alice@example.com", *email)
}

// The conflict target is partial (deleted IS NULL): a soft-deleted user who
// still authenticates at the IdP gets a fresh row and id, and their history
// stays on the dead row.
func TestUserUpsert_SoftDeletedUserGetsFreshRow(t *testing.T) {
	env := newTestEnv(t)
	first := upsert(t, env, "Alice", "idp-sub-1", "alice@example.com")

	_, err := env.superPool.Exec(context.Background(),
		`UPDATE dcim.users SET deleted = now() WHERE id = $1`, first.ID)
	require.NoError(t, err)

	second := upsert(t, env, "Alice", "idp-sub-1", "alice@example.com")

	assert.NotEqual(t, first.ID, second.ID)
	_, _, deleted := userByID(t, env, first.ID)
	assert.NotNil(t, deleted, "the dead row keeps its history")
	_, _, alive := userByID(t, env, second.ID)
	assert.Nil(t, alive)
}
