package registry_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func svgIcon(shape string) []byte {
	return []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><%s/></svg>`, shape))
}

func pngIcon(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	return buf.Bytes()
}

func iconManifest(icon, iconDark, menuIcon string) string {
	manifest := `apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: postgres-operator
  version: 1.2.3
`
	if icon != "" {
		manifest += "  icon: " + icon + "\n"
	}
	if iconDark != "" {
		manifest += "  iconDark: " + iconDark + "\n"
	}
	manifest += `spec:
  image: ghcr.io/example/postgres-operator@sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae
`
	if menuIcon != "" {
		manifest += `  menu:
    organization:
      - crd: databases.example.io
        list: true
        icon: ` + menuIcon + "\n"
	}
	return manifest
}

func filesOf(contents ...[]byte) map[string][]byte {
	files := map[string][]byte{}
	for _, content := range contents {
		files[pluginruntime.IconDigest(content)] = content
	}
	return files
}

func pushIcons(ctx context.Context, client registryClient, pluginID, manifest string, files map[string][]byte, replace bool) error {
	_, err := client.CreatePluginVersion(ctx, registryv1.CreatePluginVersionRequest_builder{
		PluginId:  pluginID,
		Version:   "v1.2.3",
		Manifest:  []byte(manifest),
		IconFiles: files,
		Replace:   replace,
	}.Build())
	return err //nolint:wrapcheck // tests assert on the connect error
}

func getPlugin(t *testing.T, client registryClient, pluginID string) *registryv1.Plugin {
	t.Helper()

	resp, err := client.GetPlugin(context.Background(), registryv1.GetPluginRequest_builder{PluginId: pluginID}.Build())
	require.NoError(t, err)
	return resp.GetPlugin()
}

func storedMenuIcons(t *testing.T, env *testEnv, pluginID string) string {
	t.Helper()

	var menuIcons string
	require.NoError(t, env.adminPool.QueryRow(context.Background(),
		`SELECT menu_icons::text FROM appstore.plugin_definitions WHERE plugin_id = $1 AND deleted IS NULL`, pluginID).Scan(&menuIcons))
	return menuIcons
}

func TestCreatePluginVersion_StoresIcons(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	logo, dark, menu := pngIcon(t), svgIcon("circle"), svgIcon("rect")
	manifest := iconManifest(pluginruntime.IconDigest(logo), pluginruntime.IconDigest(dark), pluginruntime.IconDigest(menu))
	require.NoError(t, pushIcons(context.Background(), client, plugin.GetId(), manifest, filesOf(logo, dark, menu), false))

	got := getPlugin(t, client, plugin.GetId())
	assert.Contains(t, got.GetLogo(), "data:image/png;base64,")
	assert.Contains(t, got.GetLogoDark(), "data:image/svg+xml;base64,")
	assert.Contains(t, storedMenuIcons(t, env, plugin.GetId()), pluginruntime.IconDigest(menu))
}

func TestCreatePluginVersion_RejectsBadIcons(t *testing.T) {
	logo := svgIcon("circle")
	digest := pluginruntime.IconDigest(logo)
	unsafe := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)

	tests := []struct {
		name     string
		manifest string
		files    map[string][]byte
	}{
		{"digest mismatch", iconManifest(digest, "", ""), map[string][]byte{digest: svgIcon("rect")}},
		{"missing file", iconManifest(digest, "", ""), nil},
		{"unsafe svg", iconManifest(pluginruntime.IconDigest(unsafe), "", ""), filesOf(unsafe)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			_, client := newOrgClient(t, env)
			plugin := createPlugin(t, client, "postgres-operator")

			err := pushIcons(context.Background(), client, plugin.GetId(), tt.manifest, tt.files, false)
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))

			var count int
			require.NoError(t, env.adminPool.QueryRow(context.Background(),
				`SELECT count(*) FROM appstore.plugin_definitions WHERE plugin_id = $1`, plugin.GetId()).Scan(&count))
			assert.Zero(t, count)
		})
	}
}

func TestCreatePluginVersion_NoIcons(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	require.NoError(t, pushIcons(context.Background(), client, plugin.GetId(), iconManifest("", "", ""), nil, false))

	got := getPlugin(t, client, plugin.GetId())
	assert.Empty(t, got.GetLogo())
	assert.Empty(t, got.GetLogoDark())
	assert.JSONEq(t, `{}`, storedMenuIcons(t, env, plugin.GetId()))
}

func TestCreatePluginVersion_ReplaceSwapsIcons(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	first, second := svgIcon("circle"), svgIcon("rect")
	ctx := context.Background()
	require.NoError(t, pushIcons(ctx, client, plugin.GetId(), iconManifest(pluginruntime.IconDigest(first), "", ""), filesOf(first), false))
	before := getPlugin(t, client, plugin.GetId()).GetLogo()
	require.NotEmpty(t, before)

	require.NoError(t, pushIcons(ctx, client, plugin.GetId(), iconManifest(pluginruntime.IconDigest(second), "", ""), filesOf(second), true))

	after := getPlugin(t, client, plugin.GetId()).GetLogo()
	assert.NotEmpty(t, after)
	assert.NotEqual(t, before, after)
}

// A JSON client's base64 makes the largest valid request about 2.1 MB, so that
// size must reach the handler's own validation rather than the transport limit.
func TestCreatePluginVersion_ReadLimitLeavesRoomForJSON(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	_, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "v1.0.0",
		Manifest: make([]byte, 2_200_000),
	}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connectCode(err))
}

func TestCreatePluginVersion_RejectsOversizedRequest(t *testing.T) {
	env := newTestEnv(t)
	_, client := newOrgClient(t, env)
	plugin := createPlugin(t, client, "postgres-operator")

	_, err := client.CreatePluginVersion(context.Background(), registryv1.CreatePluginVersionRequest_builder{
		PluginId: plugin.GetId(),
		Version:  "v1.0.0",
		Manifest: make([]byte, 4<<20),
	}.Build())
	require.Error(t, err)
	assert.Equal(t, connect.CodeResourceExhausted, connectCode(err))
}
