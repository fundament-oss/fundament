package catalog_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	catalogv1 "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

const iconsManifestHead = `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: icons
  version: v1.0.0
`

func iconsManifest(icon string, menuIcons ...string) []byte {
	manifest := iconsManifestHead
	if icon != "" {
		manifest += "  icon: '" + icon + "'\n"
	}
	manifest += "spec:\n  image: ghcr.io/fundament-oss/icons@sha256:0c9f988f29c3bbb63c61e798b8a0a2c7921e332193214d01c16f5ced4684a531\n"
	if len(menuIcons) > 0 {
		manifest += "  menu:\n    organization:\n"
		for _, icon := range menuIcons {
			manifest += "      - crd: widgets.example.io\n        list: true\n        icon: '" + icon + "'\n"
		}
	}
	return []byte(manifest)
}

// definitionWithIcons publishes manifest with the stored logo and menu icons and
// returns the definition GetPluginDefinition serves for it.
func definitionWithIcons(t *testing.T, name string, manifest []byte, logo, menuIcons string) *marketplacev1.PluginDefinition {
	t.Helper()

	env := newTestEnv(t)
	id := seedPlugin(t, env, seedOptions{Name: name, Visibility: "public", Published: true, Manifest: manifest})
	_, err := env.adminPool.Exec(t.Context(),
		`UPDATE appstore.plugin_definitions SET logo = $2, menu_icons = $3::jsonb WHERE plugin_id = $1`,
		id, logo, menuIcons)
	require.NoError(t, err)

	resp, err := newServer(t, env).GetPluginDefinition(context.Background(),
		catalogv1.GetPluginDefinitionRequest_builder{PluginId: new(id.String()), Version: "1.0.0", IncludeDefinition: true}.Build())
	require.NoError(t, err)
	require.NotNil(t, resp.GetDefinition())
	return resp.GetDefinition()
}

func TestGetPluginDefinitionResolvesIconDigests(t *testing.T) {
	logoDigest := pluginruntime.IconDigest([]byte("logo"))
	menuDigest := pluginruntime.IconDigest([]byte("menu"))

	def := definitionWithIcons(t, "icons-resolved", iconsManifest(logoDigest, menuDigest),
		"data:image/png;base64,AA", `{"`+menuDigest+`": "data:image/svg+xml;base64,BB"}`)

	assert.Equal(t, "data:image/png;base64,AA", def.GetMetadata().GetIcon())
	require.Len(t, def.GetMenu().GetOrganization(), 1)
	assert.Equal(t, "data:image/svg+xml;base64,BB", def.GetMenu().GetOrganization()[0].GetIcon())
}

func TestGetPluginDefinitionBlanksUnresolvedIconDigest(t *testing.T) {
	digest := pluginruntime.IconDigest([]byte("menu"))

	def := definitionWithIcons(t, "icons-unresolved", iconsManifest(digest, digest), "", "{}")

	assert.Empty(t, def.GetMetadata().GetIcon())
	require.Len(t, def.GetMenu().GetOrganization(), 1)
	assert.Empty(t, def.GetMenu().GetOrganization()[0].GetIcon())
}

func TestGetPluginDefinitionBlanksOddIconValues(t *testing.T) {
	for _, value := range []string{"https://x/y.png", "icons/a.svg", "data:image/svg+xml;base64,AA", "sha256:zz"} {
		t.Run(value, func(t *testing.T) {
			def := definitionWithIcons(t, "icons-odd", iconsManifest(value, value, "globe"), "", "{}")

			assert.Empty(t, def.GetMetadata().GetIcon())
			entries := def.GetMenu().GetOrganization()
			require.Len(t, entries, 2)
			assert.Empty(t, entries[0].GetIcon())
			assert.Equal(t, "globe", entries[1].GetIcon())
		})
	}
}
