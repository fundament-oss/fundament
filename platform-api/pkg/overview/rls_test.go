package overview_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
)

// These tests talk to Postgres as the public role, around the server. The
// handler tests prove what the service returns; these prove that even a
// compromised handler could read no more than the function hands out.

func isGrantDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege
}

func TestPublicRoleReadsNoTable(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	pool := env.RolePool(t, "fun_platform_public_api")

	for _, table := range []string{
		"tenant.organizations",
		"tenant.clusters",
		"tenant.projects",
		"tenant.namespaces",
		"tenant.cluster_snapshots",
		"tenant.gardener_shoots",
		"tenant.users",
	} {
		_, err := pool.Exec(t.Context(), "SELECT 1 FROM "+table+" LIMIT 1")
		require.Error(t, err, table)
		assert.True(t, isGrantDenied(err), "%s: expected insufficient_privilege, got %v", table, err)
	}

	_, err := pool.Exec(t.Context(), `INSERT INTO tenant.organizations (name, alias) VALUES ('rogue', 'Rogue')`)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got %v", err)
}

func TestPublicRoleRunsOverviewFunction(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	pool := env.RolePool(t, "fun_platform_public_api")

	var count int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM tenant.platform_overview()`).Scan(&count))
	assert.Positive(t, count)

	// The binary refuses to start against an unknown schema, so the role must
	// be able to read the migration version.
	require.NoError(t, dbversion.AssertLatestVersion(t.Context(), pool))
}
