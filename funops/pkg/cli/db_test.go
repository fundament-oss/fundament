package cli

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// newTestContext gives a command a database of its own, cloned from the
// migrated template with the test data in it, connected as a superuser the
// way funops runs against a real installation (fun_operator bypasses RLS).
func newTestContext(t *testing.T) *Context {
	t.Helper()

	name := testNameToDbName(t.Name())

	adminPool, err := pgxpool.New(t.Context(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", testDBPort))
	require.NoError(t, err)
	defer adminPool.Close()

	_, err = adminPool.Exec(t.Context(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)
	_, err = adminPool.Exec(t.Context(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	require.NoError(t, err)

	pool, err := pgxpool.New(t.Context(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, name))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return &Context{
		Output:  OutputTable,
		Logger:  slog.Default(),
		DB:      &psqldb.DB{Pool: pool},
		Queries: db.New(pool),
	}
}

// liveUsersAt counts the live rows at an address, case-insensitively: what
// users_uq_email promises there is at most one of.
func liveUsersAt(t *testing.T, ctx *Context, email string) int {
	t.Helper()

	var n int
	err := ctx.DB.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.users WHERE lower(email) = lower($1) AND deleted IS NULL`, email).Scan(&n)
	require.NoError(t, err)
	return n
}

// membershipListParamsFor resolves an organization name for MembershipList.
func membershipListParamsFor(t *testing.T, ctx *Context, organization string) db.MembershipListParams {
	t.Helper()

	id, err := lookupOrganizationID(t.Context(), ctx.Queries, organization)
	require.NoError(t, err)
	return db.MembershipListParams{OrganizationID: id}
}
