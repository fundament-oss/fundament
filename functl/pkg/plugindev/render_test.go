package plugindev

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

func renderTo(t *testing.T, l Layout) string {
	t.Helper()
	out := t.TempDir()
	require.NoError(t, Render(out, RenderParams{Layout: l, Target: Sandbox, OrgName: "acme-corp", Version: "1.2.0-dev"}))
	return out
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(b)
}

func TestRenderStandalone(t *testing.T) {
	out := renderTo(t, Layout{ModuleRoot: "/work/demo", ModulePath: "example.com/demo", Package: ".", Name: "demo", Vite: true})

	skaffold := readFile(t, out, "skaffold.yaml")
	assert.Contains(t, skaffold, `context: "/work/demo"`)
	assert.Contains(t, skaffold, `PLUGIN_PACKAGE: "."`)
	assert.Contains(t, skaffold, "kubeContext: k3d-fundament-plugin")
	rel, err := filepath.Rel("/work/demo", filepath.Join(out, "Dockerfile"))
	require.NoError(t, err)
	assert.Contains(t, skaffold, `dockerfile: "`+filepath.ToSlash(rel)+`"`)
	assert.NotContains(t, skaffold, "--build-context")
	assert.Contains(t, skaffold, `"--provenance=false"`)
	assert.Contains(t, skaffold, `"--sbom=false"`)

	ignore := readFile(t, out, "Dockerfile.dockerignore")
	assert.Contains(t, ignore, "node_modules")
	assert.NotContains(t, ignore, "console")
	assert.NotContains(t, ignore, "go.mod")

	dockerfile := readFile(t, out, "Dockerfile")
	assert.Contains(t, dockerfile, `include_dir = ["."]`)
	assert.NotContains(t, dockerfile, "GOWORK")
}

func TestRenderInRepo(t *testing.T) {
	out := renderTo(t, Layout{ModuleRoot: "/repo", ModulePath: FundamentModule, Package: "./plugins/external-dns", Name: "external-dns"})

	ignore := readFile(t, out, "Dockerfile.dockerignore")
	assert.Contains(t, ignore, "!plugin-sdk/")
	assert.Contains(t, ignore, "!plugins/")
	assert.Contains(t, ignore, "!go.mod")
	assert.Contains(t, ignore, "!go.sum")
	assert.NotContains(t, ignore, "console")

	assert.Contains(t, readFile(t, out, "Dockerfile"), `include_dir = ["plugins/external-dns", "plugin-sdk", "common"]`)
}

func TestRenderSDKReplace(t *testing.T) {
	out := renderTo(t, Layout{ModuleRoot: "/work/demo", ModulePath: "example.com/demo", Package: ".", Name: "demo", SDKReplace: "/src/fundament"})

	assert.Contains(t, readFile(t, out, "skaffold.yaml"), "fundament-sdk=/src/fundament")
	dockerfile := readFile(t, out, "Dockerfile")
	assert.Contains(t, dockerfile, "/sdk")
	assert.Contains(t, dockerfile, "GOWORK")
}

func TestRenderInstallation(t *testing.T) {
	out := renderTo(t, Layout{ModuleRoot: "/work/demo", ModulePath: "example.com/demo", Package: ".", Name: "demo"})

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
