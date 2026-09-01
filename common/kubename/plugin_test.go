package kubename

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestPluginNamespace_VerbatimWhenItFits(t *testing.T) {
	assert.Equal(t, "plugin-system--cert-manager", PluginNamespace("system--cert-manager"))
}

func TestPluginNamespace_VerbatimAtTheBoundary(t *testing.T) {
	// 56 chars + "plugin-" is exactly 63, the DNS-1123 label limit.
	name := strings.Repeat("a", 56)
	got := PluginNamespace(name)
	assert.Equal(t, "plugin-"+name, got)
	assert.Len(t, got, validation.DNS1123LabelMaxLength)
}

func TestPluginNamespace_HashesWhenTooLong(t *testing.T) {
	name := strings.Repeat("a", 57)
	got := PluginNamespace(name)
	assert.NotEqual(t, "plugin-"+name, got)
	assert.LessOrEqual(t, len(got), validation.DNS1123LabelMaxLength)
	require.Empty(t, validation.IsDNS1123Label(got))
}

func TestPluginNamespace_IsDeterministic(t *testing.T) {
	name := strings.Repeat("b", 100)
	assert.Equal(t, PluginNamespace(name), PluginNamespace(name))
}

func TestPluginNamespace_DistinguishesSharedPrefixes(t *testing.T) {
	// Both names share far more than the 47-char readable budget, so only the
	// hash can tell them apart.
	prefix := strings.Repeat("c", 60)
	assert.NotEqual(t, PluginNamespace(prefix+"-one"), PluginNamespace(prefix+"-two"))
}

func TestPluginNamespace_NoTrailingDashOnTruncation(t *testing.T) {
	// Truncation lands exactly on the dash at index 46, which is included in [:47].
	name := strings.Repeat("d", 46) + "-" + strings.Repeat("e", 20)
	got := PluginNamespace(name)
	assert.NotContains(t, got, "--")
	require.Empty(t, validation.IsDNS1123Label(got))
}
