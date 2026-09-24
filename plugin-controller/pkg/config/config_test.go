package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
