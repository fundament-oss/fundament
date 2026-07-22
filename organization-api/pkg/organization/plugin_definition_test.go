package organization_test

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

const testPluginName = "test-plugin-def"

var testManifest = []byte("apiVersion: fundament.io/v1\nkind: PluginDefinition\nmetadata:\n  name: test-plugin-def\n  version: v1\nspec:\n  image: repo@sha256:aa\n  permissions:\n    rbac:\n      - apiGroups: [cert-manager.io]\n        resources: [certificates]\n        verbs: [get]\n")

func newPluginServiceClient(env *testEnv) organizationv1connect.PluginServiceClient {
	return organizationv1connect.NewPluginServiceClient(env.server.Client(), env.server.URL)
}

// seedCatalogPlugin inserts a row into appstore.plugins and returns its id.
func seedCatalogPlugin(t *testing.T, env *testEnv, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := env.adminPool.Exec(t.Context(),
		"INSERT INTO appstore.plugins (id, name, description) VALUES ($1, $2, $3)",
		id, name, "test plugin",
	)
	require.NoError(t, err)
	return id
}

func TestPutPluginDefinition_IdempotentAndConflict(t *testing.T) {
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

	pluginID := seedCatalogPlugin(t, env, testPluginName)

	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	putReq1 := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID.String(),
		PluginVersion: "v1",
		Manifest:      testManifest,
	}.Build())
	putReq1.Header().Set("Authorization", "Bearer "+token)
	putReq1.Header().Set("Fun-Organization", orgID.String())

	resp1, err := client.PutPluginDefinition(ctx, putReq1)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(resp1.Msg.GetHash(), "sha256:"))
	assert.Equal(t, pluginID.String(), resp1.Msg.GetPluginId())

	// Same bytes → idempotent, same hash.
	putReq2 := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID.String(),
		PluginVersion: "v1",
		Manifest:      testManifest,
	}.Build())
	putReq2.Header().Set("Authorization", "Bearer "+token)
	putReq2.Header().Set("Fun-Organization", orgID.String())

	resp2, err := client.PutPluginDefinition(ctx, putReq2)
	require.NoError(t, err)
	assert.Equal(t, resp1.Msg.GetHash(), resp2.Msg.GetHash())

	// Different bytes, same (plugin_id, version) → FAILED_PRECONDITION.
	putReq3 := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID.String(),
		PluginVersion: "v1",
		Manifest:      append(testManifest, byte('\n')),
	}.Build())
	putReq3.Header().Set("Authorization", "Bearer "+token)
	putReq3.Header().Set("Fun-Organization", orgID.String())

	_, err = client.PutPluginDefinition(ctx, putReq3)
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

// TestPutPluginDefinition_RequiresOrganization verifies the endpoint is
// org-scoped: an authenticated user without a Fun-Organization header cannot
// publish a definition.
func TestPutPluginDefinition_RequiresOrganization(t *testing.T) {
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

	pluginID := seedCatalogPlugin(t, env, testPluginName)
	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)

	putReq := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID.String(),
		PluginVersion: "v1",
		Manifest:      testManifest,
	}.Build())
	putReq.Header().Set("Authorization", "Bearer "+token)
	// No Fun-Organization header.

	_, err := client.PutPluginDefinition(context.Background(), putReq)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestPutPluginDefinition_RejectsVersionMismatch verifies the request's
// plugin_version must equal the manifest's metadata.version.
func TestPutPluginDefinition_RejectsVersionMismatch(t *testing.T) {
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

	pluginID := seedCatalogPlugin(t, env, testPluginName)
	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)

	// testManifest declares metadata.version v1; send a different plugin_version.
	putReq := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID.String(),
		PluginVersion: "v2",
		Manifest:      testManifest,
	}.Build())
	putReq.Header().Set("Authorization", "Bearer "+token)
	putReq.Header().Set("Fun-Organization", orgID.String())

	_, err := client.PutPluginDefinition(context.Background(), putReq)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestPutPluginDefinition_RejectsImagelessTemplate(t *testing.T) {
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

	pluginID := seedCatalogPlugin(t, env, testPluginName)

	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	template := []byte("apiVersion: fundament.io/v1\nkind: PluginDefinition\nmetadata:\n  name: test-plugin-def\n  version: v1\nspec:\n  permissions:\n    rbac: []\n")

	putReq := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID.String(),
		PluginVersion: "v1",
		Manifest:      template,
	}.Build())
	putReq.Header().Set("Authorization", "Bearer "+token)
	putReq.Header().Set("Fun-Organization", orgID.String())

	_, err := client.PutPluginDefinition(ctx, putReq)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestPutPluginDefinition_UnknownPluginID(t *testing.T) {
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

	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	unknownID := uuid.New()
	putReq := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      unknownID.String(),
		PluginVersion: "v1",
		Manifest:      testManifest,
	}.Build())
	putReq.Header().Set("Authorization", "Bearer "+token)
	putReq.Header().Set("Fun-Organization", orgID.String())

	_, err := client.PutPluginDefinition(ctx, putReq)
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestPutPluginDefinition_InvalidPluginID(t *testing.T) {
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

	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	putReq := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      "not-a-uuid",
		PluginVersion: "v1",
		Manifest:      testManifest,
	}.Build())
	putReq.Header().Set("Authorization", "Bearer "+token)
	putReq.Header().Set("Fun-Organization", orgID.String())

	_, err := client.PutPluginDefinition(ctx, putReq)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
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

	pluginID := seedCatalogPlugin(t, env, testPluginName)

	token := env.createAuthnToken(t, userID)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	// Arrange: Put a manifest first.
	putReq := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID.String(),
		PluginVersion: "v1",
		Manifest:      testManifest,
	}.Build())
	putReq.Header().Set("Authorization", "Bearer "+token)
	putReq.Header().Set("Fun-Organization", orgID.String())

	_, err := client.PutPluginDefinition(ctx, putReq)
	require.NoError(t, err)

	// Act: Get the definition (public endpoint, no auth needed).
	getReq := connect.NewRequest(organizationv1.GetPluginDefinitionRequest_builder{
		PluginName:    testPluginName,
		PluginVersion: "v1",
	}.Build())

	resp, err := client.GetPluginDefinition(ctx, getReq)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.GetManifest())
	assert.True(t, strings.HasPrefix(resp.Msg.GetHash(), "sha256:"))
	assert.Equal(t, "repo@sha256:aa", resp.Msg.GetDefinition().GetImage())
	require.Len(t, resp.Msg.GetDefinition().GetPermissions().GetRbac(), 1)
	assert.Equal(t, []string{"cert-manager.io"}, resp.Msg.GetDefinition().GetPermissions().GetRbac()[0].GetApiGroups())
}

func TestGetPluginDefinition_NotFound(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := newPluginServiceClient(env)
	ctx := context.Background()

	getReq := connect.NewRequest(organizationv1.GetPluginDefinitionRequest_builder{
		PluginName:    "nope",
		PluginVersion: "v1",
	}.Build())

	_, err := client.GetPluginDefinition(ctx, getReq)
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
