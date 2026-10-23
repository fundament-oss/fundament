package plugindev

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

const demoDefinition = "metadata:\n  name: demo\n  version: v1.2.0\n"

func TestDetectStandalone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/demo-plugin\n\ngo 1.22\n")
	writeFile(t, filepath.Join(dir, "definition.yaml"), demoDefinition)

	l, err := Detect(dir)
	require.NoError(t, err)
	assert.Equal(t, ".", l.Package)
	assert.Equal(t, l.PluginDir, l.ModuleRoot)
	assert.Equal(t, "example.com/demo-plugin", l.ModulePath)
	assert.Equal(t, "demo", l.Name)
	assert.Equal(t, "v1.2.0", l.Version)
	assert.False(t, l.Vite)
	assert.Empty(t, l.SDKReplace)
}

func TestDetectInRepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module "+FundamentModule+"\n")
	pluginDir := filepath.Join(root, "plugins", "storage", "ceph-rook")
	writeFile(t, filepath.Join(pluginDir, "definition.yaml"), demoDefinition)

	l, err := Detect(pluginDir)
	require.NoError(t, err)
	assert.Equal(t, "./plugins/storage/ceph-rook", l.Package)
	assert.Equal(t, FundamentModule, l.ModulePath)
	assert.Equal(t, root, l.ModuleRoot)
}

func TestDetectRelativeDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module "+FundamentModule+"\n")
	writeFile(t, filepath.Join(root, "plugins", "x", "definition.yaml"), demoDefinition)
	t.Chdir(root)

	l, err := Detect("plugins/x")
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(l.PluginDir))
	assert.True(t, filepath.IsAbs(l.ModuleRoot))
	assert.Equal(t, "./plugins/x", l.Package)
}

func TestDetectVite(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/demo\n")
	writeFile(t, filepath.Join(dir, "definition.yaml"), demoDefinition)
	writeFile(t, filepath.Join(dir, "console-ui", "package.json"), "{}")

	l, err := Detect(dir)
	require.NoError(t, err)
	assert.True(t, l.Vite)
}

func TestDetectSDKReplace(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "demo")
	writeFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/demo\n\nreplace "+FundamentModule+" => ../fundament\n")
	writeFile(t, filepath.Join(dir, "definition.yaml"), demoDefinition)

	l, err := Detect(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(parent, "fundament"), l.SDKReplace)
}

func TestDetectSDKReplaceModuleVersionIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"),
		"module example.com/demo\n\nreplace "+FundamentModule+" => example.com/fork v1.0.0\n")
	writeFile(t, filepath.Join(dir, "definition.yaml"), demoDefinition)

	l, err := Detect(dir)
	require.NoError(t, err)
	assert.Empty(t, l.SDKReplace)
}

func TestDetectNoGoMod(t *testing.T) {
	// TempDir sits under the OS temp dir, which has no go.mod above it.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "definition.yaml"), demoDefinition)

	_, err := Detect(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no go.mod")
}

func TestDetectNoDefinition(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/demo\n")

	_, err := Detect(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definition.yaml")
}

func TestDetectMissingDefinitionBeforeGoMod(t *testing.T) {
	_, err := Detect(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definition.yaml")
}

func TestSourceDirs(t *testing.T) {
	got := sourceDirs("plugins/openfsc", []string{
		"plugins/openfsc", "plugins/openfsc/internal/ui",
		"plugin-sdk/pluginruntime", "plugin-sdk/pluginruntime/console",
		"openfsc-operator/api/v1",
	})
	assert.Equal(t, []string{"openfsc-operator", "plugin-sdk", "plugins/openfsc"}, got)

	// A dep elsewhere under plugins/ widens to plugins/, which covers the plugin dir.
	assert.Equal(t, []string{"plugin-sdk", "plugins"},
		sourceDirs("plugins/a", []string{"plugins/a", "plugins/shared", "plugin-sdk"}))
}

func TestResolveSourceDirsRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list on the repository")
	}
	l, err := Detect(filepath.Join("..", "..", "..", "plugins", "openfsc"))
	require.NoError(t, err)
	require.NoError(t, l.ResolveSourceDirs())
	assert.Contains(t, l.SourceDirs, "openfsc-operator")
	assert.Contains(t, l.SourceDirs, "plugin-sdk")
	assert.Contains(t, l.SourceDirs, "plugins/openfsc")
}

func TestResolveSourceDirsStandaloneNoop(t *testing.T) {
	l := Layout{ModulePath: "example.com/demo"}
	require.NoError(t, l.ResolveSourceDirs())
	assert.Empty(t, l.SourceDirs)
}
