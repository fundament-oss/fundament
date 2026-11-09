package organization_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
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

const iconsPluginName = "icons-plugin"

func iconSVG(shape string) []byte {
	return []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><%s/></svg>`, shape))
}

func iconPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	return buf.Bytes()
}

func iconsManifest(icon, menuIcon string) []byte {
	manifest := "apiVersion: fundament.io/v1\nkind: PluginDefinition\nmetadata:\n  name: " + iconsPluginName + "\n  version: v1\n"
	if icon != "" {
		manifest += "  icon: " + icon + "\n"
	}
	manifest += "spec:\n  image: repo@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n"
	if menuIcon != "" {
		manifest += "  menu:\n    organization:\n      - crd: widgets.example.io\n        list: true\n        icon: " + menuIcon + "\n"
	}
	return []byte(manifest)
}

type iconsEnv struct {
	env      *testEnv
	client   organizationv1connect.PluginServiceClient
	orgID    uuid.UUID
	pluginID uuid.UUID
	token    string
}

func newIconsEnv(t *testing.T) *iconsEnv {
	t.Helper()

	userID := uuid.New()
	orgID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", Email: "test@example.com", OrgIDs: []uuid.UUID{orgID}}),
	)
	return &iconsEnv{
		env:      env,
		client:   newPluginServiceClient(env),
		orgID:    orgID,
		pluginID: seedCatalogPlugin(t, env, iconsPluginName, orgID),
		token:    env.createAuthnToken(t, userID),
	}
}

func (e *iconsEnv) ctx() context.Context {
	ctx, info := connect.NewClientContext(context.Background())
	info.RequestHeader().Set("Authorization", "Bearer "+e.token)
	info.RequestHeader().Set("Fun-Organization", e.orgID.String())
	return ctx
}

func (e *iconsEnv) get(t *testing.T) *organizationv1.GetPluginDefinitionResponse {
	t.Helper()

	//nolint:staticcheck // covers the deprecated RPC until it is removed
	resp, err := e.client.GetPluginDefinition(e.ctx(), organizationv1.GetPluginDefinitionRequest_builder{
		OrganizationName: "test-org", PluginName: iconsPluginName, PluginVersion: "v1",
	}.Build())
	require.NoError(t, err)
	return resp
}

// insertRaw writes a definition row directly, without icon validation.
func (e *iconsEnv) insertRaw(t *testing.T, version string, manifest []byte, logo string, published bool) {
	t.Helper()

	_, err := e.env.adminPool.Exec(t.Context(),
		`INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash, logo, published)
		 VALUES ($1, $2, $3, $4, $5, CASE WHEN $6 THEN now() END)`,
		e.pluginID, version, manifest, pluginruntime.HashManifest(manifest)+version, logo, published)
	require.NoError(t, err)
}

func TestGetPluginDefinition_ResolvesMenuIcons(t *testing.T) {
	t.Parallel()

	e := newIconsEnv(t)
	logo := iconPNG(t)
	menu := iconSVG("circle")
	logoDigest, menuDigest := pluginruntime.IconDigest(logo), pluginruntime.IconDigest(menu)
	manifest := iconsManifest(logoDigest, menuDigest)
	def, err := pluginruntime.ParseDefinition(manifest)
	require.NoError(t, err)
	icons, err := pluginruntime.ValidateIcons(&def, map[string][]byte{logoDigest: logo, menuDigest: menu})
	require.NoError(t, err)
	menuJSON, err := json.Marshal(icons.MenuIcons)
	require.NoError(t, err)
	_, err = e.env.adminPool.Exec(t.Context(),
		`INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash, logo, menu_icons)
		 VALUES ($1, 'v1', $2, $3, $4, $5::jsonb)`,
		e.pluginID, manifest, pluginruntime.HashManifest(manifest), icons.Logo, string(menuJSON))
	require.NoError(t, err)

	resp := e.get(t)
	entries := resp.GetDefinition().GetMenu().GetOrganization()
	require.Len(t, entries, 1)
	assert.True(t, strings.HasPrefix(entries[0].GetIcon(), "data:image/svg+xml;base64,"))
	assert.True(t, strings.HasPrefix(resp.GetDefinition().GetMetadata().GetIcon(), "data:image/png;base64,"))
	assert.Equal(t, manifest, resp.GetManifest())
	assert.Equal(t, pluginruntime.HashManifest(resp.GetManifest()), resp.GetHash())
}

func TestGetPluginDefinition_UnresolvedDigest(t *testing.T) {
	t.Parallel()

	e := newIconsEnv(t)
	digest := pluginruntime.IconDigest(iconSVG("circle"))
	e.insertRaw(t, "v1", iconsManifest("", digest), "", false)

	resp := e.get(t)
	require.Len(t, resp.GetDefinition().GetMenu().GetOrganization(), 1)
	assert.Empty(t, resp.GetDefinition().GetMenu().GetOrganization()[0].GetIcon())
}

func TestGetPluginDefinition_OddStoredValue(t *testing.T) {
	t.Parallel()

	odd := []string{"https://x/y.png", "icons/a.svg", "data:image/svg+xml;base64,AA", "sha256:zz"}
	for _, value := range odd {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			e := newIconsEnv(t)
			manifest := string(iconsManifest("'"+value+"'", "'"+value+"'")) +
				"      - crd: gadgets.example.io\n        list: true\n        icon: globe\n"
			e.insertRaw(t, "v1", []byte(manifest), "", false)

			resp := e.get(t)
			assert.Empty(t, resp.GetDefinition().GetMetadata().GetIcon())
			entries := resp.GetDefinition().GetMenu().GetOrganization()
			require.Len(t, entries, 2)
			assert.Empty(t, entries[0].GetIcon())
			assert.Equal(t, "globe", entries[1].GetIcon())
		})
	}
}

func listedLogo(t *testing.T, e *iconsEnv) (string, bool) {
	t.Helper()

	resp, err := e.client.ListPlugins(e.ctx(), organizationv1.ListPluginsRequest_builder{}.Build()) //nolint:staticcheck // covers the deprecated RPC until it is removed
	require.NoError(t, err)
	for _, p := range resp.GetPlugins() {
		if p.GetName() == iconsPluginName {
			return p.GetLogo(), true
		}
	}
	return "", false
}

func TestListPlugins_LogoFromLatestPublished(t *testing.T) {
	t.Parallel()

	e := newIconsEnv(t)
	manifest := iconsManifest("", "")
	e.insertRaw(t, "v1", manifest, "data:logo-a", true)
	e.insertRaw(t, "v2", manifest, "data:logo-b", false)

	logo, found := listedLogo(t, e)
	require.True(t, found)
	assert.Equal(t, "data:logo-a", logo)
}

func TestListPlugins_IgnoresDeletedPublished(t *testing.T) {
	t.Parallel()

	e := newIconsEnv(t)
	manifest := iconsManifest("", "")
	e.insertRaw(t, "v1", manifest, "data:logo-a", true)
	e.insertRaw(t, "v2", manifest, "data:logo-deleted", true)
	_, err := e.env.adminPool.Exec(t.Context(),
		"UPDATE appstore.plugin_definitions SET deleted = now() WHERE plugin_id = $1 AND plugin_version = 'v2'", e.pluginID)
	require.NoError(t, err)

	logo, found := listedLogo(t, e)
	require.True(t, found)
	assert.Equal(t, "data:logo-a", logo)
}

func TestListPlugins_NoPublishedVersion(t *testing.T) {
	t.Parallel()

	e := newIconsEnv(t)
	e.insertRaw(t, "v1", iconsManifest("", ""), "data:logo-draft", false)

	logo, found := listedLogo(t, e)
	require.True(t, found)
	assert.Empty(t, logo)
}
