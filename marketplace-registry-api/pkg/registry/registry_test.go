package registry_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
)

// seededOrganizationID is the organization db/seed/0100-system-org.sql writes.
// Tests deliberately do not publish into it: db/seed/0101-appstore-catalog.sql
// already gives it sixteen listings, including cert-manager, so creating there
// collides on plugins_uq_name and makes any count assertion depend on the seed.
var seededOrganizationID = uuid.MustParse("019b4000-0000-7000-8000-000000000000")

// newOrgClient returns a client acting for an organization of its own, so a
// test starts from an empty appstore.
func newOrgClient(t *testing.T, env *testEnv) (uuid.UUID, registryClient) {
	t.Helper()

	orgID := seedOrganization(t, env, "org-"+uuid.NewString()[:8])
	userID := seedUser(t, env, "user-"+uuid.NewString()[:8])
	return orgID, newClient(t, env, authFor(t, userID, orgID))
}

// A published definition must carry a digest-pinned image or ParseDefinition
// rejects it, so the fixture pins one.
const testManifest = `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: postgres-operator
  version: 1.2.3
spec:
  image: ghcr.io/example/postgres-operator@sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae
`

func seedOrganization(t *testing.T, env *testEnv, name string) uuid.UUID {
	t.Helper()

	orgID := uuid.New()
	_, err := env.adminPool.Exec(context.Background(),
		`INSERT INTO tenant.organizations (id, name, alias) VALUES ($1, $2, $2)`, orgID, name)
	require.NoError(t, err)

	return orgID
}

// seedUser inserts the user the token will name. submissions.submitter_user_id
// carries an FK to tenant.users, so a token for an id that was never inserted
// only fails once a version is submitted.
func seedUser(t *testing.T, env *testEnv, name string) uuid.UUID {
	t.Helper()

	userID := uuid.New()
	_, err := env.adminPool.Exec(context.Background(),
		`INSERT INTO tenant.users (id, name) VALUES ($1, $2)`, userID, name)
	require.NoError(t, err)

	return userID
}

func createPlugin(t *testing.T, client registryClient, name string) *registryv1.Plugin {
	t.Helper()

	resp, err := client.CreatePlugin(context.Background(), registryv1.CreatePluginRequest_builder{
		Name:        name,
		DisplayName: "Postgres Operator",
		Description: "Runs Postgres.",
		Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
	}.Build())
	require.NoError(t, err)

	return resp.GetPlugin()
}

func TestCreatePluginReservesTheNameAndReadsBack(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	assert.Equal(t, "postgres-operator", plugin.GetName())
	assert.NotEmpty(t, plugin.GetOrganizationId())
	assert.Equal(t, registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC, plugin.GetVisibility())
	// Nothing is approved yet, so the listing is not live.
	assert.Empty(t, plugin.GetLatestPublishedVersionId())
}

func TestCreatePluginRejectsADuplicateNameInTheSameOrganization(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	createPlugin(t, client, "postgres-operator")

	_, err := client.CreatePlugin(context.Background(), registryv1.CreatePluginRequest_builder{
		Name:        "postgres-operator",
		DisplayName: "Another One",
		Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connectCode(err))
}

// The proto's DNS-1123 rule permits a double dash but plugins_ck_name does not,
// because that is the <organization>--<plugin> separator. Without the handler's
// own check this surfaces as an opaque check violation.
func TestCreatePluginRejectsADoubleDashInTheName(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	_, err := client.CreatePlugin(context.Background(), registryv1.CreatePluginRequest_builder{
		Name:        "postgres--operator",
		DisplayName: "Postgres Operator",
		Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))
}

// plugins_ck_name spells the first and last character separately, so it needs
// two of them; the proto's DNS-1123 rule is happy with one.
func TestCreatePluginRejectsASingleCharacterName(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	_, err := client.CreatePlugin(context.Background(), registryv1.CreatePluginRequest_builder{
		Name:        "a",
		DisplayName: "A",
		Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))
}

// Two organizations may each publish a plugin called cert-manager (FUN-20).
func TestCreatePluginAllowsTheSameNameInAnotherOrganization(t *testing.T) {
	env := newTestEnv(t)
	_, first := newOrgClient(t, env)
	secondOrgID, second := newOrgClient(t, env)

	createPlugin(t, first, "cert-manager")
	plugin := createPlugin(t, second, "cert-manager")

	assert.Equal(t, secondOrgID.String(), plugin.GetOrganizationId())
}

func TestListPluginsReturnsOnlyTheCallersOrganization(t *testing.T) {
	env := newTestEnv(t)
	_, mine := newOrgClient(t, env)
	_, theirs := newOrgClient(t, env)

	createPlugin(t, mine, "postgres-operator")
	createPlugin(t, theirs, "cert-manager")

	resp, err := mine.ListPlugins(context.Background(), registryv1.ListPluginsRequest_builder{}.Build())
	require.NoError(t, err)

	require.Len(t, resp.GetPlugins(), 1, "a caller must not see another organization's listings")
	assert.Equal(t, "postgres-operator", resp.GetPlugins()[0].GetName())
}

// One query per kind covers the whole page, so every child row has to be routed
// back to its own listing.
func TestListPluginsKeepsChildRowsWithTheirOwnListing(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	first := createPlugin(t, client, "postgres-operator")
	second := createPlugin(t, client, "cert-manager")

	_, err := client.UpdatePlugin(context.Background(), registryv1.UpdatePluginRequest_builder{
		PluginId:    first.GetId(),
		DisplayName: "Postgres Operator",
		Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
		Tags:        []string{"database"},
		Features: []*marketplacev1.FeatureBlock{
			marketplacev1.FeatureBlock_builder{Title: "Backups", Body: "Nightly."}.Build(),
		},
	}.Build())
	require.NoError(t, err)

	_, err = client.UpdatePlugin(context.Background(), registryv1.UpdatePluginRequest_builder{
		PluginId:    second.GetId(),
		DisplayName: "Cert Manager",
		Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
		Tags:        []string{"certificates", "tls"},
		DocumentationLinks: []*marketplacev1.DocumentationLink{
			marketplacev1.DocumentationLink_builder{Title: "Guide", UrlName: "guide", Url: "https://example.com/guide"}.Build(),
		},
	}.Build())
	require.NoError(t, err)

	resp, err := client.ListPlugins(context.Background(), registryv1.ListPluginsRequest_builder{}.Build())
	require.NoError(t, err)

	byName := map[string]*registryv1.Plugin{}
	for _, plugin := range resp.GetPlugins() {
		byName[plugin.GetName()] = plugin
	}
	require.Len(t, byName, 2)

	assert.Equal(t, []string{"database"}, byName["postgres-operator"].GetTags())
	require.Len(t, byName["postgres-operator"].GetFeatures(), 1)
	assert.Equal(t, "Backups", byName["postgres-operator"].GetFeatures()[0].GetTitle())
	assert.Empty(t, byName["postgres-operator"].GetDocumentationLinks())

	assert.Equal(t, []string{"certificates", "tls"}, byName["cert-manager"].GetTags())
	require.Len(t, byName["cert-manager"].GetDocumentationLinks(), 1)
	assert.Equal(t, "Guide", byName["cert-manager"].GetDocumentationLinks()[0].GetTitle())
	assert.Empty(t, byName["cert-manager"].GetFeatures())
}

// RLS scopes the role to the caller's organization, so another organization's
// plugin resolves to nothing and is indistinguishable from one that never
// existed.
func TestGetPluginHidesAnotherOrganizationsListing(t *testing.T) {
	env := newTestEnv(t)
	_, theirs := newOrgClient(t, env)
	plugin := createPlugin(t, theirs, "cert-manager")

	_, mine := newOrgClient(t, env)
	_, err := mine.GetPlugin(context.Background(), registryv1.GetPluginRequest_builder{
		PluginId: plugin.GetId(),
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connectCode(err))
}

func TestUpdatePluginReplacesRepeatedFieldsWholesale(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	update := func(tags []string) *registryv1.Plugin {
		resp, err := client.UpdatePlugin(context.Background(), registryv1.UpdatePluginRequest_builder{
			PluginId:    plugin.GetId(),
			DisplayName: "Postgres Operator",
			Tags:        tags,
			Features: []*marketplacev1.FeatureBlock{
				marketplacev1.FeatureBlock_builder{Title: "Backups", Body: "Nightly."}.Build(),
			},
			Visibility: registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
		}.Build())
		require.NoError(t, err)
		return resp.GetPlugin()
	}

	assert.Equal(t, []string{"database", "operator"}, update([]string{"database", "operator"}).GetTags())

	// Omitting a tag clears it rather than merging with what was there.
	after := update([]string{"database"})
	assert.Equal(t, []string{"database"}, after.GetTags())
	require.Len(t, after.GetFeatures(), 1, "features are replaced, not appended")
}

func TestUpdatePluginStoresTheAllowListForRestrictedListings(t *testing.T) {
	env := newTestEnv(t)
	allowedOrgID := seedOrganization(t, env, "acme")
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	resp, err := client.UpdatePlugin(context.Background(), registryv1.UpdatePluginRequest_builder{
		PluginId:               plugin.GetId(),
		DisplayName:            "Postgres Operator",
		Visibility:             registryv1.PluginVisibility_PLUGIN_VISIBILITY_RESTRICTED,
		AllowedOrganizationIds: []string{allowedOrgID.String()},
	}.Build())
	require.NoError(t, err)

	assert.Equal(t, registryv1.PluginVisibility_PLUGIN_VISIBILITY_RESTRICTED, resp.GetPlugin().GetVisibility())
	assert.Equal(t, []string{allowedOrgID.String()}, resp.GetPlugin().GetAllowedOrganizationIds())
}

// updateLinks applies a documentation-link set to a listing, holding the rest of
// the request steady so only the links vary.
// updateChildrenRequest holds the scalar fields steady so only the child rows
// under test vary between calls.
func updateChildrenRequest(pluginID string, links []*marketplacev1.DocumentationLink, features []*marketplacev1.FeatureBlock) *registryv1.UpdatePluginRequest {
	return registryv1.UpdatePluginRequest_builder{
		PluginId:           pluginID,
		DisplayName:        "Postgres Operator",
		Visibility:         registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
		DocumentationLinks: links,
		Features:           features,
	}.Build()
}

func updateLinksRequest(pluginID string, links ...*marketplacev1.DocumentationLink) *registryv1.UpdatePluginRequest {
	return updateChildrenRequest(pluginID, links, nil)
}

func updateFeaturesRequest(pluginID string, features ...*marketplacev1.FeatureBlock) *registryv1.UpdatePluginRequest {
	return updateChildrenRequest(pluginID, nil, features)
}

func updateLinks(t *testing.T, client registryClient, pluginID string, links ...*marketplacev1.DocumentationLink) *registryv1.Plugin {
	t.Helper()

	resp, err := client.UpdatePlugin(context.Background(), updateLinksRequest(pluginID, links...))
	require.NoError(t, err)

	return resp.GetPlugin()
}

func updateFeatures(t *testing.T, client registryClient, pluginID string, features ...*marketplacev1.FeatureBlock) *registryv1.Plugin {
	t.Helper()

	resp, err := client.UpdatePlugin(context.Background(), updateFeaturesRequest(pluginID, features...))
	require.NoError(t, err)

	return resp.GetPlugin()
}

// A documentation link is the one child row whose id the API hands out, so an
// update carrying that id must land on the same row instead of replacing it
// with a fresh one; a client keying on the id breaks otherwise.
func TestUpdatePluginKeepsTheIDOfADocumentationLinkHandedBack(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	created := updateLinks(t, client, plugin.GetId(), marketplacev1.DocumentationLink_builder{
		Title: "Guides", UrlName: "Getting started", Url: "https://example.com/start",
	}.Build())
	require.Len(t, created.GetDocumentationLinks(), 1)

	linkID := created.GetDocumentationLinks()[0].GetId()
	require.NotEmpty(t, linkID)

	edited := updateLinks(t, client, plugin.GetId(), marketplacev1.DocumentationLink_builder{
		Id: linkID, Title: "Guides", UrlName: "Quickstart", Url: "https://example.com/start",
	}.Build())
	require.Len(t, edited.GetDocumentationLinks(), 1)

	assert.Equal(t, linkID, edited.GetDocumentationLinks()[0].GetId(), "an id handed back must survive the update")
	assert.Equal(t, "Quickstart", edited.GetDocumentationLinks()[0].GetUrlName(), "the row is edited in place")
}

// Honouring ids does not make the field additive: a link the request leaves out
// is still dropped.
func TestUpdatePluginDropsADocumentationLinkTheRequestOmits(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	created := updateLinks(t, client, plugin.GetId(),
		marketplacev1.DocumentationLink_builder{Title: "Guides", UrlName: "Start", Url: "https://example.com/start"}.Build(),
		marketplacev1.DocumentationLink_builder{Title: "Guides", UrlName: "API", Url: "https://example.com/api"}.Build(),
	)
	require.Len(t, created.GetDocumentationLinks(), 2)

	keptID := created.GetDocumentationLinks()[0].GetId()

	after := updateLinks(t, client, plugin.GetId(), marketplacev1.DocumentationLink_builder{
		Id: keptID, Title: "Guides", UrlName: "Start", Url: "https://example.com/start",
	}.Build())

	require.Len(t, after.GetDocumentationLinks(), 1)
	assert.Equal(t, keptID, after.GetDocumentationLinks()[0].GetId())
}

// The update is scoped by plugin_id as well as id, so an id from somewhere else
// is an error rather than a silent no-op. The whole request rolls back with it.
func TestUpdatePluginRejectsAnUnknownDocumentationLinkID(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	created := updateLinks(t, client, plugin.GetId(), marketplacev1.DocumentationLink_builder{
		Title: "Guides", UrlName: "Start", Url: "https://example.com/start",
	}.Build())
	require.Len(t, created.GetDocumentationLinks(), 1)

	_, err := client.UpdatePlugin(context.Background(), updateLinksRequest(plugin.GetId(),
		marketplacev1.DocumentationLink_builder{
			Id: uuid.NewString(), Title: "Guides", UrlName: "Elsewhere", Url: "https://example.com/other",
		}.Build()))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))

	resp, err := client.GetPlugin(context.Background(), registryv1.GetPluginRequest_builder{
		PluginId: plugin.GetId(),
	}.Build())
	require.NoError(t, err)
	assert.Len(t, resp.GetPlugin().GetDocumentationLinks(), 1, "a rejected update must not have dropped the existing link")
}

// A feature block carries an id for the same reason a documentation link does,
// so it follows the same rule: carried back, it edits that block in place.
func TestUpdatePluginKeepsTheIDOfAFeatureBlockHandedBack(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	created := updateFeatures(t, client, plugin.GetId(),
		marketplacev1.FeatureBlock_builder{Title: "Backups", Body: "Nightly."}.Build(),
		marketplacev1.FeatureBlock_builder{Title: "Failover", Body: "Automatic."}.Build(),
	)
	require.Len(t, created.GetFeatures(), 2)

	keptID := created.GetFeatures()[0].GetId()
	require.NotEmpty(t, keptID)

	// Carries the first back edited and omits the second.
	after := updateFeatures(t, client, plugin.GetId(), marketplacev1.FeatureBlock_builder{
		Id: keptID, Title: "Backups", Body: "Hourly.",
	}.Build())

	require.Len(t, after.GetFeatures(), 1, "an omitted block is still dropped")
	assert.Equal(t, keptID, after.GetFeatures()[0].GetId(), "an id handed back must survive the update")
	assert.Equal(t, "Hourly.", after.GetFeatures()[0].GetBody(), "the row is edited in place")
}

func TestUpdatePluginRejectsAnUnknownFeatureBlockID(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	created := updateFeatures(t, client, plugin.GetId(),
		marketplacev1.FeatureBlock_builder{Title: "Backups", Body: "Nightly."}.Build())
	require.Len(t, created.GetFeatures(), 1)

	_, err := client.UpdatePlugin(context.Background(), updateFeaturesRequest(plugin.GetId(),
		marketplacev1.FeatureBlock_builder{Id: uuid.NewString(), Title: "Backups", Body: "Nightly."}.Build()))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))

	resp, err := client.GetPlugin(context.Background(), registryv1.GetPluginRequest_builder{
		PluginId: plugin.GetId(),
	}.Build())
	require.NoError(t, err)
	assert.Len(t, resp.GetPlugin().GetFeatures(), 1, "a rejected update must not have dropped the existing block")
}

// Both entries would land on the same row, and the caller would read back fewer
// links than it sent.
func TestUpdatePluginRejectsTheSameDocumentationLinkIDTwice(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	created := updateLinks(t, client, plugin.GetId(), marketplacev1.DocumentationLink_builder{
		Title: "Guides", UrlName: "Start", Url: "https://example.com/start",
	}.Build())
	require.Len(t, created.GetDocumentationLinks(), 1)

	linkID := created.GetDocumentationLinks()[0].GetId()

	_, err := client.UpdatePlugin(context.Background(), updateLinksRequest(plugin.GetId(),
		marketplacev1.DocumentationLink_builder{Id: linkID, Title: "Guides", UrlName: "Start", Url: "https://example.com/start"}.Build(),
		marketplacev1.DocumentationLink_builder{Id: linkID, Title: "Guides", UrlName: "API", Url: "https://example.com/api"}.Build(),
	))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))
}

func TestCreatePluginVersionLandsInDraftAndDerivesTheHash(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	resp, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "v1.2.3",
		Manifest: []byte(testManifest),
	}.Build())
	require.NoError(t, err)

	version := resp.GetVersion()
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_DRAFT, version.GetStatus())
	// The client never sends either: both are read out of the manifest bytes.
	assert.Contains(t, version.GetDefinitionHash(), "sha256:")
	assert.Contains(t, version.GetImage(), "@sha256:")
	assert.Nil(t, version.GetPublished())
}

// The stored row and the bytes it pins must agree about what was published.
func TestCreatePluginVersionRejectsAManifestNamingAnotherPlugin(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "cert-manager")

	_, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "v1.2.3",
		Manifest: []byte(testManifest), // metadata.name is postgres-operator
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))
}

func TestCreatePluginVersionRejectsAVersionDisagreeingWithTheManifest(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	_, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "v9.9.9",
		Manifest: []byte(testManifest),
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))
}

// The proto accepts a leading v and a manifest may carry either form.
func TestCreatePluginVersionAcceptsAVersionWithoutTheVPrefix(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	_, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "1.2.3",
		Manifest: []byte(testManifest),
	}.Build())

	require.NoError(t, err)
}

func TestCreatePluginVersionRejectsAnUnparseableManifest(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin := createPlugin(t, client, "postgres-operator")

	_, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "v1.2.3",
		Manifest: []byte("not a plugin definition"),
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))
}

// The plugin is named after testManifest's metadata.name, which checkMatches
// insists on.
func seedDraftVersion(t *testing.T, client registryClient) (*registryv1.Plugin, *registryv1.PluginVersion) {
	t.Helper()

	plugin := createPlugin(t, client, "postgres-operator")
	resp, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "v1.2.3",
		Manifest: []byte(testManifest),
	}.Build())
	require.NoError(t, err)

	return plugin, resp.GetVersion()
}

// plugin_definitions_uq_plugin_version is on the raw text, so storing the
// version verbatim would let "v1.2.3" and "1.2.3" sit under one plugin as two
// rows claiming the same semantic version.
func TestCreatePluginVersionStoresTheVersionNormalized(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	_, version := seedDraftVersion(t, client)

	assert.Equal(t, "1.2.3", version.GetVersion())
}

func TestCreatePluginVersionRejectsTheSameVersionUnderADifferentPrefix(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin, _ := seedDraftVersion(t, client)

	// seedDraftVersion pushed "v1.2.3"; this is the same version unprefixed.
	_, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "1.2.3",
		Manifest: []byte(testManifest),
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connectCode(err))
}

func TestSubmitPluginVersionMovesDraftToPending(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	_, version := seedDraftVersion(t, client)

	resp, err := client.SubmitPluginVersion(context.Background(), registryv1.SubmitPluginVersionRequest_builder{
		PluginVersionId: version.GetId(),
	}.Build())
	require.NoError(t, err)

	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING, resp.GetVersion().GetStatus())
	assert.NotNil(t, resp.GetVersion().GetSubmitted(), "submitting records when the round opened")
}

// submissions_uq_open would refuse a second open round anyway; the handler
// returns current state rather than surfacing that as an error.
func TestSubmitPluginVersionIsIdempotentWhilePending(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	_, version := seedDraftVersion(t, client)
	submit := registryv1.SubmitPluginVersionRequest_builder{PluginVersionId: version.GetId()}.Build()

	_, err := client.SubmitPluginVersion(context.Background(), submit)
	require.NoError(t, err)

	resp, err := client.SubmitPluginVersion(context.Background(), submit)
	require.NoError(t, err)
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING, resp.GetVersion().GetStatus())
}

func TestWithdrawPluginVersionReturnsAPendingVersionToWithdrawn(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	_, version := seedDraftVersion(t, client)

	_, err := client.SubmitPluginVersion(context.Background(), registryv1.SubmitPluginVersionRequest_builder{
		PluginVersionId: version.GetId(),
	}.Build())
	require.NoError(t, err)

	resp, err := client.WithdrawPluginVersion(context.Background(), registryv1.WithdrawPluginVersionRequest_builder{
		PluginVersionId: version.GetId(),
	}.Build())
	require.NoError(t, err)

	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_WITHDRAWN, resp.GetVersion().GetStatus())
}

// A withdrawn version can be submitted again, opening a fresh round.
func TestSubmitPluginVersionAcceptsAWithdrawnVersion(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	_, version := seedDraftVersion(t, client)
	submit := registryv1.SubmitPluginVersionRequest_builder{PluginVersionId: version.GetId()}.Build()

	_, err := client.SubmitPluginVersion(context.Background(), submit)
	require.NoError(t, err)
	_, err = client.WithdrawPluginVersion(context.Background(), registryv1.WithdrawPluginVersionRequest_builder{
		PluginVersionId: version.GetId(),
	}.Build())
	require.NoError(t, err)

	resp, err := client.SubmitPluginVersion(context.Background(), submit)
	require.NoError(t, err)
	assert.Equal(t, marketplacev1.SubmissionStatus_SUBMISSION_STATUS_PENDING, resp.GetVersion().GetStatus())
}

func TestWithdrawPluginVersionRefusesADraft(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	_, version := seedDraftVersion(t, client)

	_, err := client.WithdrawPluginVersion(context.Background(), registryv1.WithdrawPluginVersionRequest_builder{
		PluginVersionId: version.GetId(),
	}.Build())

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connectCode(err))
}

// A soft-deleted listing must not keep serving its version history.
func TestDeletePluginCascadesToVersions(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	plugin, version := seedDraftVersion(t, client)

	_, err := client.DeletePlugin(context.Background(), registryv1.DeletePluginRequest_builder{
		PluginId: plugin.GetId(),
	}.Build())
	require.NoError(t, err)

	_, err = client.GetPlugin(context.Background(), registryv1.GetPluginRequest_builder{
		PluginId: plugin.GetId(),
	}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connectCode(err))

	_, err = client.GetPluginVersion(context.Background(), registryv1.GetPluginVersionRequest_builder{
		PluginVersionId: version.GetId(),
	}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connectCode(err), "a deleted listing must not keep serving its versions")
}

func TestListCategoriesServesTheCuratedVocabulary(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)

	resp, err := client.ListCategories(context.Background(), registryv1.ListCategoriesRequest_builder{}.Build())
	require.NoError(t, err)

	assert.NotEmpty(t, resp.GetCategories(), "the seed provides categories a developer can pick from")
}
