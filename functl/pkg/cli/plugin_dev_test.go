package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/functl/pkg/config"
)

func devFixture(t *testing.T, definition string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "definition.yaml"), []byte(definition), 0o600))
	return dir
}

const devDefinition = "metadata:\n  name: demo\n  version: v1.0.0\n"

func TestPluginDevNoActiveOrg(t *testing.T) {
	cmd := PluginDevCmd{Dir: devFixture(t, devDefinition)}
	err := cmd.Run(&Context{Config: &config.Config{}})
	require.ErrorIs(t, err, ErrNoActiveOrganization)
}

func TestPluginDevDownNoActiveOrg(t *testing.T) {
	cmd := PluginDevDownCmd{Dir: devFixture(t, devDefinition)}
	err := cmd.Run(&Context{Config: &config.Config{}})
	require.ErrorIs(t, err, ErrNoActiveOrganization)
}

func TestPluginDevRejectsEmptyNameOrVersion(t *testing.T) {
	dir := devFixture(t, "metadata:\n  name: demo\n")
	ctx := &Context{Config: &config.Config{Organization: "org"}}

	err := (&PluginDevCmd{Dir: dir}).Run(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata.version")

}

func TestDetectNamedVersionOptionalForDevDown(t *testing.T) {
	dir := devFixture(t, "metadata:\n  name: demo\n")
	_, err := detectNamed(dir, true)
	require.Error(t, err)
	l, err := detectNamed(dir, false)
	require.NoError(t, err)
	assert.Equal(t, "demo", l.Name)
}

func TestDetectNamedRejectsInvalidName(t *testing.T) {
	_, err := detectNamed(devFixture(t, "metadata:\n  name: ../Evil\n  version: v1.0.0\n"), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DNS label")
}

func TestDetectNamedRejectsSeparator(t *testing.T) {
	_, err := detectNamed(devFixture(t, "metadata:\n  name: my--plugin\n  version: v1.0.0\n"), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"--"`)
}

func TestDevVersion(t *testing.T) {
	assert.Equal(t, "v1.0.0-dev", devVersion("v1.0.0"))
	assert.Equal(t, "v1.0.0-dev", devVersion("v1.0.0-dev"))
}
