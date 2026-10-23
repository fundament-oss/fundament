package plugindev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

func renderTo(t *testing.T, l Layout) string {
	t.Helper()
	out := t.TempDir()
	p := RenderParams{Layout: l, Target: Sandbox, OrgName: "acme-corp"}
	require.NoError(t, Render(out, p))
	require.NoError(t, RenderInstallation(out, p, "1.2.0-dev"))
	return out
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(b)
}

func TestRenderStandalone(t *testing.T) {
	out := renderTo(t, Layout{ModuleRoot: t.TempDir(), ModulePath: "example.com/demo", Package: ".", Name: "demo", Vite: true})

	skaffold := readFile(t, out, "skaffold.yaml")
	assert.Contains(t, skaffold, `PLUGIN_PACKAGE: "."`)
	assert.Contains(t, skaffold, "kubeContext: k3d-fundament-plugin")
	assert.NotContains(t, skaffold, "--build-context")
	assert.Contains(t, skaffold, `"--provenance=false"`)
	assert.Contains(t, skaffold, `"--sbom=false"`)

	ignore := readFile(t, out, "Dockerfile.dockerignore")
	assert.Contains(t, ignore, "node_modules")
	assert.Contains(t, ignore, "!console\n!console/**")
	assert.NotContains(t, ignore, "go.mod")

	dockerfile := readFile(t, out, "Dockerfile")
	assert.Contains(t, dockerfile, `include_dir = ["."]`)
	assert.NotContains(t, dockerfile, "GOWORK")
}

func TestRenderDockerfilePath(t *testing.T) {
	root := t.TempDir()
	out := renderTo(t, Layout{ModuleRoot: root, ModulePath: "example.com/demo", Package: ".", Name: "demo"})

	skaffold := readFile(t, out, "skaffold.yaml")
	assert.Contains(t, skaffold, `context: "`+root+`"`)
	rel, err := filepath.Rel(root, filepath.Join(out, "Dockerfile"))
	require.NoError(t, err)
	assert.Contains(t, skaffold, `dockerfile: "`+filepath.ToSlash(rel)+`"`)
}

func TestRenderStandaloneKeepsProjectIgnore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".dockerignore"), ".env\nconsole/\n")
	out := renderTo(t, Layout{ModuleRoot: root, ModulePath: "example.com/demo", Package: ".", Name: "demo", Vite: true})

	ignore := readFile(t, out, "Dockerfile.dockerignore")
	assert.Contains(t, ignore, ".env\nconsole/\n")
	// The re-include must come after the project's console/ rule to win.
	assert.Greater(t, strings.Index(ignore, "!console"), strings.Index(ignore, "console/"))
}

func TestRenderInRepo(t *testing.T) {
	out := renderTo(t, Layout{
		ModuleRoot: t.TempDir(), ModulePath: FundamentModule, Package: "./plugins/openfsc", Name: "openfsc",
		SourceDirs: []string{"openfsc-operator", "plugin-sdk", "plugins/openfsc"},
	})

	ignore := readFile(t, out, "Dockerfile.dockerignore")
	assert.Contains(t, ignore, "!openfsc-operator/")
	assert.Contains(t, ignore, "!plugin-sdk/")
	assert.Contains(t, ignore, "!plugins/openfsc/")
	assert.Contains(t, ignore, "!go.mod")
	assert.Contains(t, ignore, "!go.sum")
	assert.NotContains(t, ignore, "!plugins/\n")

	assert.Contains(t, readFile(t, out, "Dockerfile"), `include_dir = ["openfsc-operator", "plugin-sdk", "plugins/openfsc"]`)
}

func TestRenderInRepoNeedsSourceDirs(t *testing.T) {
	err := Render(t.TempDir(), RenderParams{Layout: Layout{ModuleRoot: t.TempDir(), ModulePath: FundamentModule, Package: "./plugins/x", Name: "x"}})
	require.Error(t, err)
}

func TestRenderSDKReplace(t *testing.T) {
	out := renderTo(t, Layout{ModuleRoot: t.TempDir(), ModulePath: "example.com/demo", Package: ".", Name: "demo", SDKReplace: "/src/fundament"})

	assert.Contains(t, readFile(t, out, "skaffold.yaml"), "fundament-sdk=/src/fundament")
	dockerfile := readFile(t, out, "Dockerfile")
	assert.Contains(t, dockerfile, "/sdk")
	assert.Contains(t, dockerfile, "GOWORK")
}

func TestRenderInstallation(t *testing.T) {
	out := renderTo(t, Layout{ModuleRoot: t.TempDir(), ModulePath: "example.com/demo", Package: ".", Name: "demo"})

	var inst pluginsv1.PluginInstallation
	require.NoError(t, yaml.Unmarshal([]byte(readFile(t, out, "installation.yaml")), &inst))
	assert.Equal(t, "acme-corp--demo", inst.Name)
	assert.Equal(t, "true", inst.Annotations[pluginsv1.AnnotationDevMode])
	assert.Equal(t, "1.2.0-dev", inst.Spec.DefinitionRef.PluginVersion)
	assert.Equal(t, "acme-corp", inst.Spec.DefinitionRef.OrganizationName)
	require.NotNil(t, inst.Spec.Dev)
	assert.Equal(t, "demo-plugin-dev", inst.Spec.Dev.Image)
}

func TestNames(t *testing.T) {
	assert.Equal(t, "acme--demo", InstallationName("acme", "demo"))
	assert.Equal(t, "demo-plugin-dev", ImageName("demo"))
}
