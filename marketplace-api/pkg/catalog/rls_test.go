package catalog_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/fundament-oss/fundament/marketplace-api/pkg/db/gen"
)

// seedOptions describes one plugin plus its single definition.
type seedOptions struct {
	Name       string
	Visibility string
	Published  bool
	Deleted    bool
	Featured   bool
	// Defaults to a single NUL byte when empty. Set a real manifest when the
	// test needs capabilities or permissions parsed out of it.
	Manifest []byte
}

func seedPlugin(t *testing.T, env *testEnv, opts seedOptions) uuid.UUID {
	t.Helper()

	ctx := context.Background()
	admin := env.adminPool

	orgID := uuid.MustParse("019b4000-0000-7000-8000-000000000000")
	pluginID := uuid.New()

	_, err := admin.Exec(ctx, `
		INSERT INTO appstore.plugins (id, organization_id, name, display_name, description, visibility, deleted, featured)
		VALUES ($1, $2, $3, $3, 'seeded', $4, CASE WHEN $5::bool THEN now() ELSE NULL END, $6)`,
		pluginID, orgID, opts.Name, opts.Visibility, opts.Deleted, opts.Featured)
	require.NoError(t, err)

	manifest := opts.Manifest
	if manifest == nil {
		manifest = []byte{0}
	}

	_, err = admin.Exec(ctx, `
		INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash, status, published)
		VALUES ($1, '1.0.0', $2, 'sha256:seed', $3,
			CASE WHEN $4::bool THEN now() ELSE NULL END)`,
		pluginID, manifest, statusFor(opts.Published), opts.Published)
	require.NoError(t, err)

	return pluginID
}

// seedVersion adds another definition to an existing plugin.
func seedVersion(t *testing.T, env *testEnv, pluginID uuid.UUID, version string, published bool) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := env.adminPool.QueryRow(context.Background(), `
		INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash, status, published)
		VALUES ($1, $2, '\x00'::bytea, 'sha256:' || $2, $3,
			CASE WHEN $4::bool THEN now() ELSE NULL END)
		RETURNING id`,
		pluginID, version, statusFor(published), published).Scan(&id)
	require.NoError(t, err)

	return id
}

func statusFor(published bool) string {
	if published {
		return "approved"
	}
	return "draft"
}

// listAsCatalog runs PluginList through the SELECT-only role.
func listAsCatalog(t *testing.T, env *testEnv) []uuid.UUID {
	t.Helper()

	rows, err := db.New(env.catalogPool).PluginList(context.Background(), db.PluginListParams{})
	require.NoError(t, err)

	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func TestRLSHidesUnpublishedPlugin(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{Name: "rls-unpublished", Visibility: "public", Published: false})

	assert.NotContains(t, listAsCatalog(t, env), id, "a plugin with no published version must be invisible to the catalog role")
}

func TestRLSHidesRestrictedPlugin(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{Name: "rls-restricted", Visibility: "restricted", Published: true})

	assert.NotContains(t, listAsCatalog(t, env), id, "a RESTRICTED plugin must never reach the public catalog")
}

func TestRLSHidesSoftDeletedPlugin(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{Name: "rls-deleted", Visibility: "public", Published: true, Deleted: true})

	assert.NotContains(t, listAsCatalog(t, env), id, "a soft-deleted plugin must be invisible to the catalog role")
}

func TestRLSShowsPublishedPublicPlugin(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{Name: "rls-visible", Visibility: "public", Published: true})

	assert.Contains(t, listAsCatalog(t, env), id)
}
