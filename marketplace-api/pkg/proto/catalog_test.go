package proto_test

import (
	"strings"
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
)

func TestCatalogGetPluginRequestValidation(t *testing.T) {
	t.Run("accepts a uuid plugin id", func(t *testing.T) {
		req := catalogv1.GetPluginRequest_builder{PluginId: testPluginID}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid plugin id", func(t *testing.T) {
		req := catalogv1.GetPluginRequest_builder{PluginId: "grafana-loki"}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestListPluginVersionsRequestValidation(t *testing.T) {
	t.Run("accepts a uuid plugin id", func(t *testing.T) {
		req := catalogv1.ListPluginVersionsRequest_builder{PluginId: testPluginID}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid plugin id", func(t *testing.T) {
		req := catalogv1.ListPluginVersionsRequest_builder{PluginId: "grafana-loki"}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestListPluginsRequestValidation(t *testing.T) {
	t.Run("accepts an empty request", func(t *testing.T) {
		require.NoError(t, protovalidate.Validate(catalogv1.ListPluginsRequest_builder{}.Build()))
	})

	t.Run("accepts a uuid category filter", func(t *testing.T) {
		req := catalogv1.ListPluginsRequest_builder{
			CategoryId: testCategoryID,
			Query:      "grafana",
			Sort:       catalogv1.PluginSort_PLUGIN_SORT_RECENTLY_ADDED,
		}.Build()

		require.NoError(t, protovalidate.Validate(req))
	})

	t.Run("rejects a non-uuid category filter", func(t *testing.T) {
		req := catalogv1.ListPluginsRequest_builder{CategoryId: "Observability"}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})

	t.Run("rejects an over-long query", func(t *testing.T) {
		req := catalogv1.ListPluginsRequest_builder{Query: strings.Repeat("a", 201)}.Build()

		assert.Error(t, protovalidate.Validate(req))
	})
}

func TestPluginSummaryCarriesReferencesAsIDs(t *testing.T) {
	// The storefront receives identifiers only: no copied organization name,
	// no copied version string.
	summary := catalogv1.PluginSummary_builder{
		Id:              testPluginID,
		Name:            "grafana-loki",
		OrganizationId:  testOrganizationID,
		CategoryIds:     []string{testCategoryID},
		Tags:            []string{"observability", "logging"},
		LatestVersionId: testPluginVersionID,
	}.Build()

	assert.Equal(t, testOrganizationID, summary.GetOrganizationId())
	assert.Equal(t, []string{testCategoryID}, summary.GetCategoryIds())
	assert.Equal(t, []string{"observability", "logging"}, summary.GetTags())
	assert.Equal(t, testPluginVersionID, summary.GetLatestVersionId())
}

func TestPublishedVersionIsReferenceable(t *testing.T) {
	// definition_hash belongs to the version, and the version has an id so
	// PluginSummary.latest_version_id can point at it.
	version := catalogv1.PublishedVersion_builder{
		Id:             testPluginVersionID,
		Version:        "v1.17.2",
		DefinitionHash: "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
	}.Build()

	assert.Equal(t, testPluginVersionID, version.GetId())
	assert.Equal(t, "v1.17.2", version.GetVersion())
}
