package authn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShootVerifierConfig_Validate(t *testing.T) {
	valid := func() ShootVerifierConfig {
		return ShootVerifierConfig{Mode: "auto", GardenerMode: "mock", MockSecret: DefaultMockShootSecret}
	}

	for name, tc := range map[string]struct {
		mutate func(*ShootVerifierConfig)
		errMsg string
	}{
		"auto with defaults":           {func(*ShootVerifierConfig) {}, ""},
		"real Gardener":                {func(c *ShootVerifierConfig) { c.GardenerMode = "real" }, ""},
		"mock with its own secret":     {func(c *ShootVerifierConfig) { c.Mode = "mock"; c.MockSecret = "s3cret" }, ""},
		"mock with opted-in default":   {func(c *ShootVerifierConfig) { c.Mode = "mock"; c.AllowDefaultMockSecret = true }, ""},
		"mock with the public default": {func(c *ShootVerifierConfig) { c.Mode = "mock" }, "public default MOCK_SHOOT_SECRET"},
		"mock without a secret":        {func(c *ShootVerifierConfig) { c.Mode = "mock"; c.MockSecret = "" }, "needs a MOCK_SHOOT_SECRET"},
		"verifier mode typo":           {func(c *ShootVerifierConfig) { c.Mode = "Mock" }, "invalid SHOOT_VERIFIER_MODE"},
		"empty verifier mode":          {func(c *ShootVerifierConfig) { c.Mode = "" }, "invalid SHOOT_VERIFIER_MODE"},
		"gardener mode typo":           {func(c *ShootVerifierConfig) { c.GardenerMode = "Real" }, "invalid GARDENER_MODE"},
		"auto ignores the mock secret": {func(c *ShootVerifierConfig) { c.MockSecret = "" }, ""},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := valid()
			tc.mutate(&cfg)
			err := cfg.validate()
			if tc.errMsg == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
}
