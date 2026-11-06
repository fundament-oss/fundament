package organization_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

const testPluginName = "test-plugin-def"

var testManifest = []byte("apiVersion: fundament.io/v1\nkind: PluginDefinition\nmetadata:\n  name: test-plugin-def\n  version: v1\nspec:\n  image: repo@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n  permissions:\n    rbac:\n      - apiGroups: [cert-manager.io]\n        resources: [certificates]\n        verbs: [get]\n")

func newPluginServiceClient(env *testEnv) organizationv1connect.PluginServiceClient {
	return organizationv1connect.NewPluginServiceClient(env.server.Client(), env.server.URL)
}

// seedCatalogPlugin inserts a row into appstore.plugins owned by orgID and returns its id.
func seedCatalogPlugin(t *testing.T, env *testEnv, name string, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := env.adminPool.Exec(t.Context(),
		"INSERT INTO appstore.plugins (id, organization_id, name, description) VALUES ($1, $2, $3, $4)",
		id, orgID, name, "test plugin",
	)
	require.NoError(t, err)
	return id
}

// seedPluginDefinition inserts a plugin definition row and returns its hash.
func seedPluginDefinition(t *testing.T, env *testEnv, pluginID uuid.UUID, version string, manifest []byte) string {
	t.Helper()
	hash := pluginruntime.HashManifest(manifest)
	_, err := env.adminPool.Exec(t.Context(),
		"INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash) VALUES ($1, $2, $3, $4)",
		pluginID, version, manifest, hash,
	)
	require.NoError(t, err)
	return hash
}

// TestListPlugins_SurfacesLatestDefinition verifies ListPlugins reports the
// latest published version+hash (empty when none), which terraform pins on
// install. Drafts never surface.
func TestListPlugins_SurfacesLatestDefinition(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orgID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			Email:  "test@example.com",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	pluginID := seedCatalogPlugin(t, env, testPluginName, orgID)
	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	findSummary := func(t *testing.T) *organizationv1.PluginSummary {
		t.Helper()
		req := organizationv1.ListPluginsRequest_builder{}.Build()
		ctx, callInfo := connect.NewClientContext(ctx)
		callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
		callInfo.RequestHeader().Set("Fun-Organization", orgID.String())
		resp, err := client.ListPlugins(ctx, req) //nolint:staticcheck // covers the deprecated RPC until it is removed
		require.NoError(t, err)
		for _, p := range resp.GetPlugins() {
			if p.GetName() == testPluginName {
				return p
			}
		}
		t.Fatalf("plugin %q not in ListPlugins response", testPluginName)
		return nil
	}

	// No definition published yet → version/hash empty.
	before := findSummary(t)
	assert.Empty(t, before.GetPluginVersion())
	assert.Empty(t, before.GetDefinitionHash())

	// Approval is the only path that stamps published; mirror it directly.
	publish := func(version string) {
		t.Helper()
		_, err := env.adminPool.Exec(t.Context(),
			"UPDATE appstore.plugin_definitions SET status = 'approved', published = now() WHERE plugin_id = $1 AND plugin_version = $2",
			pluginID, version,
		)
		require.NoError(t, err)
	}

	v1Hash := seedPluginDefinition(t, env, pluginID, "v1", testManifest)
	v2Manifest := bytes.ReplaceAll(testManifest, []byte("version: v1"), []byte("version: v2"))
	v2Hash := seedPluginDefinition(t, env, pluginID, "v2", v2Manifest)

	// Both uploads are unreviewed drafts → still nothing to pin.
	drafts := findSummary(t)
	assert.Empty(t, drafts.GetPluginVersion())
	assert.Empty(t, drafts.GetDefinitionHash())

	// Only v1 published → v1, even though v2 was uploaded later.
	publish("v1")
	afterV1 := findSummary(t)
	assert.Equal(t, "v1", afterV1.GetPluginVersion())
	assert.Equal(t, v1Hash, afterV1.GetDefinitionHash())

	// v2 published → the newest publication wins.
	publish("v2")
	afterV2 := findSummary(t)
	assert.Equal(t, "v2", afterV2.GetPluginVersion())
	assert.Equal(t, v2Hash, afterV2.GetDefinitionHash())
}

func TestGetPluginDefinition_ReturnsBytesHashAndProto(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orgID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			Email:  "test@example.com",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	pluginID := seedCatalogPlugin(t, env, testPluginName, orgID)

	client := newPluginServiceClient(env)
	ctx := context.Background()

	seedPluginDefinition(t, env, pluginID, "v1", testManifest)

	// Act: Get the definition (public endpoint, no auth needed).
	getReq := organizationv1.GetPluginDefinitionRequest_builder{
		OrganizationName: "test-org",
		PluginName:       testPluginName,
		PluginVersion:    "v1",
	}.Build()

	resp, err := client.GetPluginDefinition(ctx, getReq) //nolint:staticcheck // covers the deprecated RPC until it is removed
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetManifest())
	assert.True(t, strings.HasPrefix(resp.GetHash(), "sha256:"))
	assert.Equal(t, "repo@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", resp.GetDefinition().GetImage())
	require.Len(t, resp.GetDefinition().GetPermissions().GetRbac(), 1)
	assert.Equal(t, []string{"cert-manager.io"}, resp.GetDefinition().GetPermissions().GetRbac()[0].GetApiGroups())
}

func TestGetPluginDefinition_NotFound(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	getReq := organizationv1.GetPluginDefinitionRequest_builder{
		OrganizationName: "nope",
		PluginName:       "nope",
		PluginVersion:    "v1",
	}.Build()

	_, err := client.GetPluginDefinition(ctx, getReq) //nolint:staticcheck // covers the deprecated RPC until it is removed
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// TestListPlugins_GlobalVisibility verifies a plugin owned by one org is still
// listed for a member of a different org (catalog visibility is not org-scoped).
func TestListPlugins_GlobalVisibility(t *testing.T) {
	t.Parallel()

	ownerOrgID := uuid.New()
	otherOrgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(ownerOrgID, "owner-org"),
		WithOrganization(otherOrgID, "other-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "other-user",
			Email:  "other@example.com",
			OrgIDs: []uuid.UUID{otherOrgID},
		}),
	)

	seedCatalogPlugin(t, env, testPluginName, ownerOrgID)
	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)

	req := organizationv1.ListPluginsRequest_builder{}.Build()
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", otherOrgID.String())
	resp, err := client.ListPlugins(listCtx, req) //nolint:staticcheck // covers the deprecated RPC until it is removed
	require.NoError(t, err)

	var found bool
	for _, p := range resp.GetPlugins() {
		if p.GetName() == testPluginName {
			found = true
		}
	}
	assert.True(t, found, "plugin owned by another org must still be visible in the catalog")
}

func TestGetPluginDefinition_ResolvesWithinOrganization(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orgAID := uuid.New()
	orgBID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgAID, "org-a"),
		WithOrganization(orgBID, "org-b"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			Email:  "test@example.com",
			OrgIDs: []uuid.UUID{orgAID, orgBID},
		}),
	)

	pluginAID := seedCatalogPlugin(t, env, "plugin-from-a", orgAID)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	manifestA := bytes.ReplaceAll(testManifest, []byte("name: test-plugin-def"), []byte("name: plugin-from-a"))
	seedPluginDefinition(t, env, pluginAID, "v1", manifestA)

	// The owning organization resolves it.
	got, err := client.GetPluginDefinition(ctx, organizationv1.GetPluginDefinitionRequest_builder{ //nolint:staticcheck // covers the deprecated RPC until it is removed
		OrganizationName: "org-a",
		PluginName:       "plugin-from-a",
		PluginVersion:    "v1",
	}.Build())
	require.NoError(t, err)
	assert.Equal(t, manifestA, got.GetManifest())

	// A different organization does not — the name alone is no longer the key.
	_, err = client.GetPluginDefinition(ctx, organizationv1.GetPluginDefinitionRequest_builder{ //nolint:staticcheck // covers the deprecated RPC until it is removed
		OrganizationName: "org-b",
		PluginName:       "plugin-from-a",
		PluginVersion:    "v1",
	}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// TestGetPluginDefinition_SameNameAcrossOrganizations proves the headline
// capability of org-qualified plugin identity: two different organizations
// can each publish a plugin named "shared-name", and GetPluginDefinition
// resolves each (org, name) pair to its own distinct, correctly attributed
// definition.
func TestGetPluginDefinition_SameNameAcrossOrganizations(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orgAID := uuid.New()
	orgBID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgAID, "org-alpha"),
		WithOrganization(orgBID, "org-beta"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			Email:  "test@example.com",
			OrgIDs: []uuid.UUID{orgAID, orgBID},
		}),
	)

	// The same plugin name, published by two different organizations.
	pluginA := seedCatalogPlugin(t, env, "shared-name", orgAID)
	pluginB := seedCatalogPlugin(t, env, "shared-name", orgBID)

	client := newPluginServiceClient(env)
	ctx := context.Background()

	// Give B a distinct byte payload (trailing comment) so the two definitions differ.
	manifestA := bytes.ReplaceAll(testManifest, []byte("name: test-plugin-def"), []byte("name: shared-name"))
	manifestB := append(append([]byte{}, manifestA...), []byte("# org-b\n")...)
	seedPluginDefinition(t, env, pluginA, "v1", manifestA)
	seedPluginDefinition(t, env, pluginB, "v1", manifestB)

	gotA, err := client.GetPluginDefinition(ctx, organizationv1.GetPluginDefinitionRequest_builder{ //nolint:staticcheck // covers the deprecated RPC until it is removed
		OrganizationName: "org-alpha", PluginName: "shared-name", PluginVersion: "v1",
	}.Build())
	require.NoError(t, err)

	gotB, err := client.GetPluginDefinition(ctx, organizationv1.GetPluginDefinitionRequest_builder{ //nolint:staticcheck // covers the deprecated RPC until it is removed
		OrganizationName: "org-beta", PluginName: "shared-name", PluginVersion: "v1",
	}.Build())
	require.NoError(t, err)

	// Same name, different publishers, different definitions.
	assert.Equal(t, manifestA, gotA.GetManifest())
	assert.Equal(t, manifestB, gotB.GetManifest())
	assert.NotEqual(t, gotA.GetHash(), gotB.GetHash())
}

func TestGetPluginDefinition_RequiresOrganizationName(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	env := newTestAPI(t, WithOrganization(orgID, "test-org"))

	_, err := newPluginServiceClient(env).GetPluginDefinition(context.Background(), //nolint:staticcheck // covers the deprecated RPC until it is removed
		organizationv1.GetPluginDefinitionRequest_builder{
			PluginName:    testPluginName,
			PluginVersion: "v1",
		}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
