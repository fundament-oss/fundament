package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/functl/pkg/scaffold"
)

// The default Kind is derived from the plugin name, and a name may start with a
// digit while a Kind may not. Without a prefix the only way past the error is to
// know about --kind, which the minimal template never prompts for.
func TestPluginCreateDefaultKind(t *testing.T) {
	for _, name := range []string{"cert-manager", "1password", "9-lives"} {
		t.Run(name, func(t *testing.T) {
			cmd := PluginCreateCmd{
				Name:       name,
				Yes:        true,
				Dir:        t.TempDir(),
				SDKVersion: scaffold.DefaultSDKVersion,
			}
			opts, err := cmd.resolve()
			require.NoError(t, err)

			_, err = scaffold.Generate(opts)
			require.NoError(t, err, "default kind %q for name %q", opts.Kind, name)
		})
	}
}

func TestTitleCaseIdentifier(t *testing.T) {
	assert.Equal(t, "CertManager", titleCaseIdentifier("cert-manager"))
	assert.Equal(t, "Plugin1password", titleCaseIdentifier("1password"))
	assert.Equal(t, "Plugin9Lives", titleCaseIdentifier("9-lives"))
}
