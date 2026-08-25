package catalog_test

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-api/pkg/catalog"
	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
)

func newServer(t *testing.T, env *testEnv) *catalog.Server {
	t.Helper()

	database, err := catalog.NewDB(context.Background(), slog.Default(), psqldb.Config{
		URL: catalogDSN(env.dbName),
	})
	require.NoError(t, err)
	t.Cleanup(database.Close)

	return catalog.New(slog.Default(), database, nil)
}

func TestListCategoriesReturnsSeededCategories(t *testing.T) {
	env := newTestEnv(t)

	resp, err := newServer(t, env).ListCategories(context.Background(), &catalogv1.ListCategoriesRequest{})
	require.NoError(t, err)

	names := make([]string, 0, len(resp.GetCategories()))
	for _, category := range resp.GetCategories() {
		names = append(names, category.GetName())
	}

	assert.Contains(t, names, "Observability")
}

func TestListPublishersExcludesOrgsWithoutLiveListing(t *testing.T) {
	env := newTestEnv(t)
	// Seed data has no published plugins until Task 9, so no org would
	// otherwise have a live listing; seed our own instead of modifying db/seed/.
	seedPlugin(t, env, seedOptions{Name: "publisher-listing", Visibility: "public", Published: true})

	// Two ways an org fails the live-listing test, one per half of the RLS
	// predicate: owning nothing at all, and owning only unpublished drafts.
	emptyOrgID := seedOrganization(t, env, "publisher-empty")
	draftOrgID := seedOrganization(t, env, "publisher-draft")
	seedPlugin(t, env, seedOptions{
		Name: "publisher-draft-listing", Visibility: "public", Published: false, OrganizationID: &draftOrgID,
	})

	resp, err := newServer(t, env).ListPublishers(context.Background(), &catalogv1.ListPublishersRequest{})
	require.NoError(t, err)

	require.NotEmpty(t, resp.GetPublishers(), "a published public plugin gives its org a live listing")

	ids := make([]string, 0, len(resp.GetPublishers()))
	for _, publisher := range resp.GetPublishers() {
		assert.NotEmpty(t, publisher.GetId())
		ids = append(ids, publisher.GetId())
	}
	assert.Contains(t, ids, seededOrganizationID.String(),
		"the organization owning the seeded plugin must appear in the publisher list")
	assert.NotContains(t, ids, emptyOrgID.String(),
		"an organization with no plugins must never appear in the publisher list")
	assert.NotContains(t, ids, draftOrgID.String(),
		"an organization whose only plugin is unpublished must never appear in the publisher list")
}

func TestListPluginsFiltersByQuery(t *testing.T) {
	env := newTestEnv(t)

	resp, err := newServer(t, env).ListPlugins(context.Background(),
		catalogv1.ListPluginsRequest_builder{Query: "zzz-no-such-plugin"}.Build())
	require.NoError(t, err)

	assert.Empty(t, resp.GetPlugins())
}

// LIKE metacharacters in a search term must match literally. Unescaped, "%"
// and "_" are wildcards that match every listing instead of none.
func TestListPluginsTreatsWildcardsInQueryAsLiterals(t *testing.T) {
	env := newTestEnv(t)
	seedPlugin(t, env, seedOptions{Name: "wildcard-escape", Visibility: "public", Published: true})
	server := newServer(t, env)

	for _, query := range []string{"%", "_", "\\"} {
		resp, err := server.ListPlugins(context.Background(),
			catalogv1.ListPluginsRequest_builder{Query: query}.Build())
		require.NoError(t, err)

		assert.Empty(t, resp.GetPlugins(), "%q must match literally, not as a wildcard", query)
	}
}

func TestListPluginsSortsByNameAscending(t *testing.T) {
	env := newTestEnv(t)
	// Seed data has no published plugins until Task 9, so RLS would otherwise
	// hide everything; seed our own instead of modifying db/seed/.
	seedPlugin(t, env, seedOptions{Name: "aaa-sort-test", Visibility: "public", Published: true})
	seedPlugin(t, env, seedOptions{Name: "mmm-sort-test", Visibility: "public", Published: true})
	seedPlugin(t, env, seedOptions{Name: "zzz-sort-test", Visibility: "public", Published: true})

	resp, err := newServer(t, env).ListPlugins(context.Background(),
		catalogv1.ListPluginsRequest_builder{
			Sort: catalogv1.PluginSort_PLUGIN_SORT_NAME,
		}.Build())
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetPlugins())

	names := make([]string, 0, len(resp.GetPlugins()))
	for _, plugin := range resp.GetPlugins() {
		names = append(names, plugin.GetDisplayName())
	}
	assert.IsIncreasing(t, names)
}

func TestListPluginsPopulatesLatestVersionID(t *testing.T) {
	env := newTestEnv(t)
	seedPlugin(t, env, seedOptions{Name: "latest-version-id", Visibility: "public", Published: true})

	resp, err := newServer(t, env).ListPlugins(context.Background(), &catalogv1.ListPluginsRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetPlugins())

	assert.NotEmpty(t, resp.GetPlugins()[0].GetLatestVersionId())
}

// The newest version is not necessarily the newest *published* version — an
// unreviewed push must never become what the storefront points at.
func TestListPluginsLatestVersionIgnoresUnpublishedNewerVersion(t *testing.T) {
	env := newTestEnv(t)
	pluginID := seedPlugin(t, env, seedOptions{Name: "latest-version", Visibility: "public", Published: true})
	publishedID := seedVersion(t, env, pluginID, "2.0.0", true)
	seedVersion(t, env, pluginID, "3.0.0", false)

	resp, err := newServer(t, env).ListPlugins(context.Background(),
		catalogv1.ListPluginsRequest_builder{Query: "latest-version"}.Build())
	require.NoError(t, err)
	require.Len(t, resp.GetPlugins(), 1)

	assert.Equal(t, publishedID.String(), resp.GetPlugins()[0].GetLatestVersionId())
}

func TestGetPluginReturnsDetails(t *testing.T) {
	env := newTestEnv(t)
	server := newServer(t, env)
	// Seed data has no published plugins until Task 9, so ListPlugins would
	// otherwise return empty; seed our own instead of modifying db/seed/. A
	// real manifest is required: GetPlugin parses it for capabilities and
	// permissions, and the default seed manifest (a single NUL byte) doesn't
	// parse.
	seedPlugin(t, env, seedOptions{
		Name:       "get-plugin-details",
		Visibility: "public",
		Published:  true,
		Manifest:   []byte(testManifest),
	})

	list, err := server.ListPlugins(context.Background(), &catalogv1.ListPluginsRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, list.GetPlugins())

	id := list.GetPlugins()[0].GetId()

	resp, err := server.GetPlugin(context.Background(),
		catalogv1.GetPluginRequest_builder{PluginId: id}.Build())
	require.NoError(t, err)

	assert.Equal(t, id, resp.GetPlugin().GetId())
	assert.NotEmpty(t, resp.GetPlugin().GetDescription())
}

func TestGetPluginNotFoundForHiddenPlugin(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{Name: "get-restricted", Visibility: "restricted", Published: true})

	_, err := newServer(t, env).GetPlugin(context.Background(),
		catalogv1.GetPluginRequest_builder{PluginId: id.String()}.Build())
	require.Error(t, err)

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// Capabilities and permissions are parsed out of the pinned manifest rather
// than stored, so a real manifest is the only way to test them. spec.image
// must be a digest reference (repo@sha256:<64 hex chars>) or ParseDefinition
// rejects it — the digest below has no meaning beyond satisfying that regex.
const testManifest = `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: seeded
  displayName: Seeded
  version: v1.0.0
  description: Fixture plugin.
  author: Fundament
  license: Apache-2.0
spec:
  image: ghcr.io/fundament-oss/seeded@sha256:0c9f988f29c3bbb63c61e798b8a0a2c7921e332193214d01c16f5ced4684a531
  permissions:
    capabilities:
      - internet_access
    rbac:
      - apiGroups:
          - cert-manager.io
        resources:
          - certificates
        verbs:
          - get
          - list
          - create
`

func TestGetPluginDerivesCapabilitiesFromManifest(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{
		Name:       "manifest-derived",
		Visibility: "public",
		Published:  true,
		Manifest:   []byte(testManifest),
	})

	resp, err := newServer(t, env).GetPlugin(context.Background(),
		catalogv1.GetPluginRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)

	assert.Equal(t, []string{"internet_access"}, resp.GetPlugin().GetCapabilities())

	require.Len(t, resp.GetPlugin().GetPermissions(), 1)
	assert.Equal(t, "certificates", resp.GetPlugin().GetPermissions()[0].GetResource())
	assert.Equal(t, "Read and write", resp.GetPlugin().GetPermissions()[0].GetAccess(),
		"a rule carrying create must read as write access")
}

func TestListPluginVersionsReturnsOnlyPublished(t *testing.T) {
	env := newTestEnv(t)
	server := newServer(t, env)
	// Seed data has no published plugins until Task 9, so ListPlugins would
	// otherwise return empty; seed our own instead of modifying db/seed/.
	seedPlugin(t, env, seedOptions{Name: "list-plugin-versions", Visibility: "public", Published: true})

	list, err := server.ListPlugins(context.Background(), &catalogv1.ListPluginsRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, list.GetPlugins())

	id := list.GetPlugins()[0].GetId()

	resp, err := server.ListPluginVersions(context.Background(),
		catalogv1.ListPluginVersionsRequest_builder{PluginId: id}.Build())
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetVersions())

	for _, version := range resp.GetVersions() {
		assert.NotNil(t, version.GetPublished(), "an unpublished version must never be returned")
		assert.NotEmpty(t, version.GetDefinitionHash())
	}
}

// A RESTRICTED plugin must not leak its version history even though
// GetPlugin correctly hides the plugin itself — ListPluginVersions queries
// appstore.plugin_definitions directly, so this is a separate RLS surface.
// Published: true so there genuinely is a published definition to leak.
func TestListPluginVersionsHidesRestrictedPlugin(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{Name: "versions-restricted", Visibility: "restricted", Published: true})

	resp, err := newServer(t, env).ListPluginVersions(context.Background(),
		catalogv1.ListPluginVersionsRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)

	assert.Empty(t, resp.GetVersions(), "a RESTRICTED plugin's versions must never leak through ListPluginVersions")
}

// Same leak class as above, but for a soft-deleted plugin: taking a listing
// down must also take its version history down, not just the plugin record.
func TestListPluginVersionsHidesSoftDeletedPlugin(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{Name: "versions-deleted", Visibility: "public", Published: true, Deleted: true})

	resp, err := newServer(t, env).ListPluginVersions(context.Background(),
		catalogv1.ListPluginVersionsRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)

	assert.Empty(t, resp.GetVersions(), "a soft-deleted plugin's versions must never leak through ListPluginVersions")
}

// A manifest from a newer plugin-sdk costs the page its capabilities and
// permissions, not the listing itself.
func TestGetPluginDegradesOnUnparseableManifest(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{
		Name:       "unparseable-manifest",
		Visibility: "public",
		Published:  true,
		Manifest:   []byte(testManifest + "  fieldFromANewerSdk: true\n"),
	})

	resp, err := newServer(t, env).GetPlugin(context.Background(),
		catalogv1.GetPluginRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)

	assert.Equal(t, id.String(), resp.GetPlugin().GetId())
	assert.NotEmpty(t, resp.GetPlugin().GetDescription(), "column-backed detail must survive an unparseable manifest")
	assert.Empty(t, resp.GetPlugin().GetCapabilities())
	assert.Empty(t, resp.GetPlugin().GetPermissions())
}

// bind and escalate touch no object, but a plugin holding them on a
// ClusterRole can grant itself anything.
const escalationManifest = `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: seeded
  version: v1.0.0
spec:
  image: ghcr.io/fundament-oss/seeded@sha256:0c9f988f29c3bbb63c61e798b8a0a2c7921e332193214d01c16f5ced4684a531
  permissions:
    rbac:
      - apiGroups:
          - rbac.authorization.k8s.io
        resources:
          - clusterroles
        verbs:
          - bind
          - escalate
`

func TestGetPluginTreatsEscalationVerbsAsWrite(t *testing.T) {
	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{
		Name:       "escalation-verbs",
		Visibility: "public",
		Published:  true,
		Manifest:   []byte(escalationManifest),
	})

	resp, err := newServer(t, env).GetPlugin(context.Background(),
		catalogv1.GetPluginRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)

	require.Len(t, resp.GetPlugin().GetPermissions(), 1)
	assert.Equal(t, "Read and write", resp.GetPlugin().GetPermissions()[0].GetAccess())
}
