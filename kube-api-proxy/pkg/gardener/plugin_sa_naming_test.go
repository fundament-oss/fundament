package gardener

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPluginSANames(t *testing.T) {
	// The SA is constant-named inside the plugin's namespace; only the namespace
	// is derived. Asserted against literals rather than against
	// kubename.PluginNamespace, because a wrapper compared to the helper it calls
	// can never disagree with it.
	assert.Equal(t, "plugin", pluginSAName)
	assert.Equal(t, "plugin-acme--cert-manager", pluginSANamespace("acme--cert-manager"))
}

func TestPluginSANamespace_LongNameStaysALabel(t *testing.T) {
	// An installation name may run to 239 chars, but a namespace is a 63-char
	// DNS label, so long names fall back to a truncated, hashed form.
	long := "acme-" + strings.Repeat("c", 200)
	ns := pluginSANamespace(long)

	assert.LessOrEqual(t, len(ns), 63)
	assert.True(t, strings.HasPrefix(ns, "plugin-acme-cccc"), "got %q", ns)
	assert.NotEqual(t, "plugin-"+long, ns)
}
