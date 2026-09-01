package registry_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests go around the server entirely and talk to Postgres as the
// registry's own role. The handler tests prove the service refuses; these prove
// the database refuses on its own, so a future query that forgets to scope
// still cannot reach another organization's rows.

func registryConn(t *testing.T, env *testEnv, organizationID uuid.UUID) *pgx.Conn {
	t.Helper()

	conn, err := pgx.Connect(context.Background(), registryDSN(env.dbName))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	_, err = conn.Exec(context.Background(),
		`SELECT set_config('app.current_organization_id', $1, false)`, organizationID.String())
	require.NoError(t, err)

	return conn
}

func isRLSDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege
}

// seedPluginAs inserts a listing owned by the given organization, bypassing RLS.
func seedPluginAs(t *testing.T, env *testEnv, organizationID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	pluginID := uuid.New()
	_, err := env.adminPool.Exec(context.Background(), `
		INSERT INTO appstore.plugins (id, organization_id, name, display_name, description, visibility)
		VALUES ($1, $2, $3, $3, '', 'public')`, pluginID, organizationID, name)
	require.NoError(t, err)

	return pluginID
}

func TestRLSHidesAnotherOrganizationsPlugin(t *testing.T) {
	env := newTestEnv(t)
	otherOrgID := seedOrganization(t, env, "acme")
	theirPluginID := seedPluginAs(t, env, otherOrgID, "cert-manager")

	conn := registryConn(t, env, seededOrganizationID)

	var count int
	err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM appstore.plugins WHERE id = $1`, theirPluginID).Scan(&count)
	require.NoError(t, err)

	assert.Zero(t, count, "another organization's listing must not be visible")
}

func TestRLSRefusesWritingAnotherOrganizationsPlugin(t *testing.T) {
	env := newTestEnv(t)
	otherOrgID := seedOrganization(t, env, "acme")
	theirPluginID := seedPluginAs(t, env, otherOrgID, "cert-manager")

	conn := registryConn(t, env, seededOrganizationID)

	// The UPDATE matches no visible row rather than erroring, which is the point:
	// the row is unreachable, so the write silently affects nothing.
	tag, err := conn.Exec(context.Background(),
		`UPDATE appstore.plugins SET display_name = 'hijacked' WHERE id = $1`, theirPluginID)
	require.NoError(t, err)
	assert.Zero(t, tag.RowsAffected(), "a write must not reach another organization's listing")
}

// The insert policy's WITH CHECK is what stops a caller planting a listing in
// someone else's organization.
func TestRLSRefusesInsertingIntoAnotherOrganization(t *testing.T) {
	env := newTestEnv(t)
	otherOrgID := seedOrganization(t, env, "acme")

	conn := registryConn(t, env, seededOrganizationID)

	_, err := conn.Exec(context.Background(), `
		INSERT INTO appstore.plugins (organization_id, name, display_name, description, visibility)
		VALUES ($1, 'smuggled', 'Smuggled', '', 'public')`, otherOrgID)

	require.Error(t, err)
	assert.True(t, isRLSDenied(err), "expected SQLSTATE 42501, got %v", err)
}

// Separate USING and WITH CHECK expressions are what make this fail; with only
// USING, an owner could move their plugin to another organization.
func TestRLSRefusesTransferringAPluginToAnotherOrganization(t *testing.T) {
	env := newTestEnv(t)
	otherOrgID := seedOrganization(t, env, "acme")
	myPluginID := seedPluginAs(t, env, seededOrganizationID, "postgres-operator")

	conn := registryConn(t, env, seededOrganizationID)

	_, err := conn.Exec(context.Background(),
		`UPDATE appstore.plugins SET organization_id = $1 WHERE id = $2`, otherOrgID, myPluginID)

	require.Error(t, err)
	assert.True(t, isRLSDenied(err), "expected SQLSTATE 42501, got %v", err)
}

// plugin_definitions reaches the owning organization through its plugin, so a
// version of someone else's listing is unreachable even holding its id.
func TestRLSHidesAnotherOrganizationsVersions(t *testing.T) {
	env := newTestEnv(t)
	otherOrgID := seedOrganization(t, env, "acme")
	theirPluginID := seedPluginAs(t, env, otherOrgID, "cert-manager")

	_, err := env.adminPool.Exec(context.Background(), `
		INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash, status)
		VALUES ($1, '1.0.0', '\x00', 'sha256:test', 'draft')`, theirPluginID)
	require.NoError(t, err)

	conn := registryConn(t, env, seededOrganizationID)

	var count int
	err = conn.QueryRow(context.Background(),
		`SELECT count(*) FROM appstore.plugin_definitions WHERE plugin_id = $1`, theirPluginID).Scan(&count)
	require.NoError(t, err)

	assert.Zero(t, count, "a restricted listing's version history must not leak")
}

// With the GUC unset every policy compares against NULL, which never matches:
// the role fails closed rather than seeing everything.
func TestRLSShowsNothingWithoutAnOrganizationContext(t *testing.T) {
	env := newTestEnv(t)
	seedPluginAs(t, env, seededOrganizationID, "postgres-operator")

	conn, err := pgx.Connect(context.Background(), registryDSN(env.dbName))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	var count int
	err = conn.QueryRow(context.Background(), `SELECT count(*) FROM appstore.plugins`).Scan(&count)
	require.NoError(t, err, fmt.Sprintf("querying without an organization context must not error"))

	assert.Zero(t, count, "an unset organization must reveal nothing")
}
