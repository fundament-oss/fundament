package plugindev

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const configDefinition = `metadata:
  name: demo
spec:
  configSchema:
    - name: MON_COUNT
      type: int
      default: "3"
    - name: ADMIN_EMAIL
      displayName: Admin email
      type: string
      required: true
      description: Receives alerts.
    - name: REGION
      type: string
      required: true
`

func TestResolveConfigSetOverridesFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "dev.yaml")
	writeFile(t, file, "MON_COUNT: 1\nADMIN_EMAIL: file@example.com\nREGION: nl-1\n")

	got, err := ResolveConfig([]byte(configDefinition), file, map[string]string{"ADMIN_EMAIL": "set@example.com"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"MON_COUNT": "1", "ADMIN_EMAIL": "set@example.com", "REGION": "nl-1"}, got)
}

func TestResolveConfigListsAllMissingRequired(t *testing.T) {
	_, err := ResolveConfig([]byte(configDefinition), "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_EMAIL (Admin email): Receives alerts.")
	assert.Contains(t, err.Error(), "REGION")
	assert.Contains(t, err.Error(), "--set KEY=VALUE")
}

func TestResolveConfigRejectsInvalidValue(t *testing.T) {
	_, err := ResolveConfig([]byte(configDefinition), "", map[string]string{
		"ADMIN_EMAIL": "a@example.com", "REGION": "nl-1", "MON_COUNT": "many",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MON_COUNT")
}

func TestResolveConfigRejectsUndeclaredKey(t *testing.T) {
	_, err := ResolveConfig([]byte(configDefinition), "", map[string]string{
		"ADMIN_EMAIL": "a@example.com", "REGION": "nl-1", "TYPO": "x",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TYPO")
}

func TestResolveConfigNoSchema(t *testing.T) {
	got, err := ResolveConfig([]byte("metadata:\n  name: demo\n"), "", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestResolveConfigBadFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "dev.yaml")
	writeFile(t, file, "- not\n- a map\n")
	_, err := ResolveConfig([]byte(configDefinition), file, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KEY: value")
}
