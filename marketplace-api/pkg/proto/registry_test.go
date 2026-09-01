package proto_test

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	registryv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/registry/v1"
)

func TestCreatePluginRequestValidation(t *testing.T) {
	t.Run("accepts a DNS-1123 slug with a display name", func(t *testing.T) {
		req := registryv1.CreatePluginRequest_builder{
			Name:        "postgres-operator",
			DisplayName: "Postgres Operator",
			CategoryIds: []string{testCategoryID},
			Visibility:  registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
		}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a slug with an underscore", func(t *testing.T) {
		req := registryv1.CreatePluginRequest_builder{
			Name:        "postgres_operator",
			DisplayName: "Postgres Operator",
			CategoryIds: []string{testCategoryID},
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})

	t.Run("rejects an empty display name", func(t *testing.T) {
		req := registryv1.CreatePluginRequest_builder{
			Name:        "postgres-operator",
			DisplayName: "",
			CategoryIds: []string{testCategoryID},
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestCreatePluginVersionRequestValidation(t *testing.T) {
	t.Run("accepts a semver version with a manifest", func(t *testing.T) {
		req := registryv1.CreatePluginVersionRequest_builder{
			PluginId: testPluginID,
			Version:  "v1.17.2",
			Manifest: []byte("apiVersion: fundament.io/v1\nkind: PluginDefinition\n"),
		}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("accepts a semver version without a leading v", func(t *testing.T) {
		req := registryv1.CreatePluginVersionRequest_builder{
			PluginId: testPluginID,
			Version:  "2.3.1",
			Manifest: []byte("apiVersion: fundament.io/v1\n"),
		}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-semver version", func(t *testing.T) {
		req := registryv1.CreatePluginVersionRequest_builder{
			PluginId: testPluginID,
			Version:  "latest",
			Manifest: []byte("apiVersion: fundament.io/v1\n"),
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})

	t.Run("rejects an empty manifest", func(t *testing.T) {
		req := registryv1.CreatePluginVersionRequest_builder{
			PluginId: testPluginID,
			Version:  "v1.0.0",
			Manifest: nil,
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid plugin id", func(t *testing.T) {
		req := registryv1.CreatePluginVersionRequest_builder{
			PluginId: "postgres-operator",
			Version:  "v1.0.0",
			Manifest: []byte("apiVersion: fundament.io/v1\n"),
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestUpdatePluginRequestValidation(t *testing.T) {
	t.Run("accepts a restricted plugin with an organization allow-list", func(t *testing.T) {
		req := registryv1.UpdatePluginRequest_builder{
			PluginId:               testPluginID,
			DisplayName:            "Postgres Operator",
			CategoryIds:            []string{testCategoryID},
			Visibility:             registryv1.PluginVisibility_PLUGIN_VISIBILITY_RESTRICTED,
			AllowedOrganizationIds: []string{testOrganizationID},
		}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid entry in the allow-list", func(t *testing.T) {
		req := registryv1.UpdatePluginRequest_builder{
			PluginId:               testPluginID,
			DisplayName:            "Postgres Operator",
			CategoryIds:            []string{testCategoryID},
			Visibility:             registryv1.PluginVisibility_PLUGIN_VISIBILITY_RESTRICTED,
			AllowedOrganizationIds: []string{"acme-corp"},
		}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestPluginCarriesReferencesAsIDs(t *testing.T) {
	// One organization is attached to a plugin, so the field needs no
	// qualifier — it matches appstore.plugins.organization_id.
	plugin := registryv1.Plugin_builder{
		Id:                       testPluginID,
		Name:                     "postgres-operator",
		DisplayName:              "Postgres Operator",
		OrganizationId:           testOrganizationID,
		CategoryIds:              []string{testCategoryID},
		Tags:                     []string{"database", "operator"},
		LatestPublishedVersionId: testPluginVersionID,
	}.Build()

	assert.Equal(t, testOrganizationID, plugin.GetOrganizationId())
	assert.Equal(t, []string{testCategoryID}, plugin.GetCategoryIds())
	assert.Equal(t, []string{"database", "operator"}, plugin.GetTags())
	assert.Equal(t, testPluginVersionID, plugin.GetLatestPublishedVersionId())
}
