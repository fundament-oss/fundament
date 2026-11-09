package fleet_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
)

// These tests talk to Postgres as the admin role, around the server: the role
// reads every organization and can write nothing, whatever a handler does.

func isGrantDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege
}

func TestAdminRoleReadsAcrossOrganizations(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	first := seed.Cluster(seed.Organization("first-org", "First"), "first")
	second := seed.Cluster(seed.Organization("second-org", "Second"), "second")

	var count int
	err := env.RolePool(t, "fun_platform_admin_api").QueryRow(t.Context(),
		`SELECT count(*) FROM tenant.clusters WHERE id = ANY($1)`, []any{first, second}).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestAdminRoleCannotWrite(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	seed := env.Seed(t)
	org := seed.Organization("write-org", "Write")
	cluster := seed.Cluster(org, "target")
	pool := env.RolePool(t, "fun_platform_admin_api")

	for name, stmt := range map[string]string{
		"update cluster":        `UPDATE tenant.clusters SET name = 'renamed' WHERE id = $1`,
		"delete cluster":        `DELETE FROM tenant.clusters WHERE id = $1`,
		"insert snapshot":       `INSERT INTO tenant.cluster_snapshots (cluster_id) VALUES ($1)`,
		"update organization":   `UPDATE tenant.organizations SET alias = 'x' WHERE id = (SELECT organization_id FROM tenant.clusters WHERE id = $1)`,
		"delete gardener shoot": `DELETE FROM tenant.gardener_shoots WHERE cluster_id = $1`,
	} {
		_, err := pool.Exec(t.Context(), stmt, cluster)
		require.Error(t, err, name)
		assert.True(t, isGrantDenied(err), "%s: expected insufficient_privilege, got %v", name, err)
	}
}

// Only the tables the fleet view needs are open to it.
func TestAdminRoleCannotReadUsers(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)
	_, err := env.RolePool(t, "fun_platform_admin_api").Exec(t.Context(), `SELECT 1 FROM tenant.users LIMIT 1`)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got %v", err)
}
