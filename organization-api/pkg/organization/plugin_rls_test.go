package organization_test

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

func Test_Plugins_RLS_Policies(t *testing.T) {
	t.Parallel()

	ownerOrgID := uuid.New()
	otherOrgID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(ownerOrgID, "owner-org"),
		WithOrganization(otherOrgID, "other-org"),
	)

	pluginID := seedCatalogPlugin(t, env, testPluginName, ownerOrgID)

	// Connect as the RLS-subject role (fun_fundament_api) to this test's DB.
	dbName := testNameToDbName(t.Name())
	conn, err := pgx.Connect(t.Context(),
		fmt.Sprintf("postgres://fun_fundament_api@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close(context.Background()) })

	setOrg := func(orgID string) {
		_, err := conn.Exec(t.Context(), "SELECT set_config('app.current_organization_id', $1, false)", orgID)
		require.NoError(t, err)
	}
	insertDef := func() error {
		_, err := conn.Exec(t.Context(),
			"INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash) VALUES ($1, 'v1', $2, 'sha256:x')",
			pluginID, []byte("m"))
		return err
	}
	isRLSDenied := func(err error) bool {
		var pgErr *pgconn.PgError
		return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege
	}

	// Wrong org → INSERT rejected by RLS.
	setOrg(otherOrgID.String())
	err = insertDef()
	require.Error(t, err)
	assert.True(t, isRLSDenied(err), "expected 42501, got %v", err)

	// Owner org → INSERT allowed.
	setOrg(ownerOrgID.String())
	require.NoError(t, insertDef())

	// GUC unset → global SELECT still returns the plugin row.
	_, err = conn.Exec(t.Context(), "RESET app.current_organization_id")
	require.NoError(t, err)
	var cnt int
	require.NoError(t, conn.QueryRow(t.Context(),
		"SELECT count(*) FROM appstore.plugins WHERE id = $1", pluginID).Scan(&cnt))
	assert.Equal(t, 1, cnt, "SELECT must stay global regardless of org context")

	// No-transfer: owner cannot reassign the plugin to another org (UPDATE WITH CHECK).
	setOrg(ownerOrgID.String())
	_, err = conn.Exec(t.Context(),
		"UPDATE appstore.plugins SET organization_id = $1 WHERE id = $2", otherOrgID, pluginID)
	require.Error(t, err)
	assert.True(t, isRLSDenied(err), "expected 42501 on ownership transfer, got %v", err)
}
