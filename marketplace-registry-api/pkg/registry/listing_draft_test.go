package registry_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/pluginlisting"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
)

// pushVersion pushes manifest as version and returns the version's id.
func pushVersion(t *testing.T, client registryClient, pluginID, version, manifest string) string {
	t.Helper()

	resp, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: pluginID,
		Version:  version,
		Manifest: []byte(manifest),
	}.Build())
	require.NoError(t, err)
	return resp.GetVersion().GetId()
}

// approveVersion submits a version and approves it the way
// marketplace-admin-api does, so the draft frozen on the round goes live.
func approveVersion(t *testing.T, env *testEnv, client registryClient, versionID string) {
	t.Helper()
	ctx := context.Background()

	_, err := client.SubmitPluginVersion(ctx, registryv1.SubmitPluginVersionRequest_builder{
		PluginVersionId: versionID,
	}.Build())
	require.NoError(t, err)

	tx, err := env.adminPool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `UPDATE appstore.plugin_definitions SET status = 'approved', published = now() WHERE id = $1`, versionID)
	require.NoError(t, err)
	require.NoError(t, pluginlisting.Sync(ctx, tx, uuid.MustParse(versionID)))
	_, err = tx.Exec(ctx, `UPDATE appstore.submissions SET closed = now() WHERE plugin_definition_id = $1 AND closed IS NULL`, versionID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
}

// approveListing pushes and approves a new newest version of plugin, and
// returns the listing as it reads afterwards. The manifest keeps the plugin's
// tags, which approval syncs from it.
func approveListing(t *testing.T, env *testEnv, client registryClient, plugin *registryv1.Plugin) *registryv1.Plugin {
	t.Helper()
	ctx := context.Background()

	versions, err := client.ListPluginVersions(ctx, registryv1.ListPluginVersionsRequest_builder{
		PluginId: plugin.GetId(),
	}.Build())
	require.NoError(t, err)

	version := fmt.Sprintf("2.0.%d", len(versions.GetVersions()))
	metadata := "version: " + version
	if len(plugin.GetTags()) > 0 {
		metadata += "\n  tags: [" + strings.Join(plugin.GetTags(), ", ") + "]"
	}
	manifest := strings.NewReplacer(
		"name: postgres-operator", "name: "+plugin.GetName(),
		"version: 1.2.3", metadata,
	).Replace(testManifest)
	approveVersion(t, env, client, pushVersion(t, client, plugin.GetId(), version, manifest))

	return getPlugin(t, client, plugin.GetId())
}

func updatePlugin(t *testing.T, client registryClient, req *registryv1.UpdatePluginRequest) *registryv1.Plugin {
	t.Helper()

	resp, err := client.UpdatePlugin(context.Background(), req)
	require.NoError(t, err)
	return resp.GetPlugin()
}

// Listing edits wait for review: the live listing keeps its fields until a
// version submitted after them is approved.
func TestUpdatePluginHoldsListingEditsUntilApproval(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	got := updatePlugin(t, client, registryv1.UpdatePluginRequest_builder{
		PluginId:    plugin.GetId(),
		Description: "A long write-up.",
		AuthorName:  "Example",
		Image:       "ghcr.io/example/postgres-operator:1.2.3",
		Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
	}.Build())

	assert.Equal(t, plugin.GetDescription(), got.GetDescription(), "the live listing is unchanged")
	require.NotNil(t, got.GetPendingListing())
	assert.Equal(t, "A long write-up.", got.GetPendingListing().GetDescription())
	assert.Equal(t, "Example", got.GetPendingListing().GetAuthorName())

	approved := approveListing(t, env, client, plugin)

	assert.Equal(t, "A long write-up.", approved.GetDescription())
	assert.Equal(t, "Example", approved.GetAuthorName())
	assert.Equal(t, "ghcr.io/example/postgres-operator:1.2.3", approved.GetImage())
	assert.Nil(t, approved.GetPendingListing(), "an applied draft is cleared")
}

// Visibility and the allow-list are not reviewed, and a request that changes
// nothing else leaves no draft behind.
func TestUpdatePluginAppliesVisibilityAtOnce(t *testing.T) {
	env := newTestEnv(t)
	allowedOrgID := seedOrganization(t, env, "acme")
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	got := updatePlugin(t, client, registryv1.UpdatePluginRequest_builder{
		PluginId:               plugin.GetId(),
		Description:            plugin.GetDescription(),
		Visibility:             registryv1.PluginVisibility_PLUGIN_VISIBILITY_RESTRICTED,
		AllowedOrganizationIds: []string{allowedOrgID.String()},
	}.Build())

	assert.Equal(t, registryv1.PluginVisibility_PLUGIN_VISIBILITY_RESTRICTED, got.GetVisibility())
	assert.Equal(t, []string{allowedOrgID.String()}, got.GetAllowedOrganizationIds())
	assert.Nil(t, got.GetPendingListing())
}

// The round carries the draft as it stood at submission; edits made while it
// is in review wait for the next one.
func TestSubmitPluginVersionFreezesTheDraft(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	describe := func(description string) {
		updatePlugin(t, client, registryv1.UpdatePluginRequest_builder{
			PluginId:    plugin.GetId(),
			Description: description,
			Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
		}.Build())
	}

	describe("Submitted.")
	versionID := pushVersion(t, client, plugin.GetId(), "1.2.3", testManifest)
	_, err := client.SubmitPluginVersion(context.Background(), registryv1.SubmitPluginVersionRequest_builder{
		PluginVersionId: versionID,
	}.Build())
	require.NoError(t, err)
	describe("Edited during review.")

	tx, err := env.adminPool.Begin(context.Background())
	require.NoError(t, err)
	_, err = tx.Exec(context.Background(), `UPDATE appstore.plugin_definitions SET status = 'approved', published = now() WHERE id = $1`, versionID)
	require.NoError(t, err)
	require.NoError(t, pluginlisting.Sync(context.Background(), tx, uuid.MustParse(versionID)))
	require.NoError(t, tx.Commit(context.Background()))

	got := getPlugin(t, client, plugin.GetId())
	assert.Equal(t, "Submitted.", got.GetDescription())
	require.NotNil(t, got.GetPendingListing())
	assert.Equal(t, "Edited during review.", got.GetPendingListing().GetDescription())
}

func TestUpdatePluginRejectsAnUnknownCategory(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	_, err := client.UpdatePlugin(context.Background(), registryv1.UpdatePluginRequest_builder{
		PluginId:    plugin.GetId(),
		CategoryIds: []string{uuid.NewString()},
		Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))
	assert.Contains(t, err.Error(), "unknown category")
}

// A new feature block has no id until the draft is approved.
func TestUpdatePluginLeavesNewRowsWithoutAnID(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	got := updatePlugin(t, client, updateFeaturesRequest(plugin.GetId(),
		marketplacev1.FeatureBlock_builder{Title: "Backups", Body: "Nightly."}.Build()))

	require.Len(t, got.GetPendingListing().GetFeatures(), 1)
	assert.Empty(t, got.GetPendingListing().GetFeatures()[0].GetId())
	assert.Empty(t, got.GetFeatures())
}
