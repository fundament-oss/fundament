package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests go around the queries entirely and talk to Postgres as
// fun_dcim_authn_api. The upsert tests prove what the role can do; these prove
// what it cannot: the column grants, not just the query's Go, are the
// identity-takeover and lockout guard.

func isGrantDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege
}

// external_ref is the login upsert key: re-pointing it would hand one user's
// directory entry (and token subject) to another identity.
func TestAuthnRoleCannotRetargetExternalRef(t *testing.T) {
	env := newTestEnv(t)
	row := upsert(t, env, "Alice", "idp-sub-1", "")

	_, err := env.servicePool.Exec(context.Background(),
		`UPDATE dcim.users SET external_ref = 'idp-sub-other' WHERE id = $1`, row.ID)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)
}

// deleted is the lockout switch, and DCIM owns it; the login service only
// reads live rows.
func TestAuthnRoleCannotSoftDelete(t *testing.T) {
	env := newTestEnv(t)
	row := upsert(t, env, "Alice", "idp-sub-1", "")

	_, err := env.servicePool.Exec(context.Background(),
		`UPDATE dcim.users SET deleted = now() WHERE id = $1`, row.ID)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)
}

// The id is the DCIM JWT subject; it is always database-generated, never
// chosen by the login service.
func TestAuthnRoleCannotChooseID(t *testing.T) {
	env := newTestEnv(t)

	_, err := env.servicePool.Exec(context.Background(),
		`INSERT INTO dcim.users (id, name, external_ref) VALUES (uuidv7(), 'Mallory', 'idp-sub-m')`)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)
}
