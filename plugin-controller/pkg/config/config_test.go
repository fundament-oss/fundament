package config

import (
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NAMESPACE", "ns")
	t.Setenv("FUNDAMENT_CLUSTER_ID", "019b4000-2000-7000-8000-000000000001")
	t.Setenv("FUNDAMENT_INSTALL_ID", "019b4000-3000-7000-8000-000000000001")
	t.Setenv("FUNDAMENT_ORGANIZATION_ID", "019b4000-0000-7000-8000-000000000001")
	t.Setenv("MARKETPLACE_CATALOG_API_URL", "http://catalog")
	t.Setenv("FUNDAMENT_AUTHN_API_URL", "http://authn")
}

func TestConfigParsesAllowDevMode(t *testing.T) {
	setRequiredEnv(t)

	var withFlag Config
	t.Setenv("PLUGIN_CONTROLLER_ALLOW_DEV_MODE", "true")
	require.NoError(t, env.Parse(&withFlag))
	assert.True(t, withFlag.AllowDevMode)

	var withoutFlag Config
	t.Setenv("PLUGIN_CONTROLLER_ALLOW_DEV_MODE", "")
	require.NoError(t, env.Parse(&withoutFlag))
	assert.False(t, withoutFlag.AllowDevMode)
}

func TestConfig_Validate(t *testing.T) {
	valid := Config{
		FundamentClusterID: "019b4000-2000-7000-8000-000000000001",
		FundamentOrgID:     "019b4000-0000-7000-8000-000000000001",
	}
	require.NoError(t, valid.Validate())

	releaseName := valid
	releaseName.FundamentClusterID = "fundament"
	err := releaseName.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FUNDAMENT_CLUSTER_ID")

	badOrg := valid
	badOrg.FundamentOrgID = "fundament"
	err = badOrg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FUNDAMENT_ORGANIZATION_ID")
}
