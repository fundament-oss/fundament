package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gardenerCadvisorSpec() map[string]any {
	return map[string]any{
		"scheme": "HTTPS",
		"relabelings": []any{
			map[string]any{"action": "replace", "replacement": "cadvisor", "targetLabel": "job"},
			map[string]any{"replacement": "shoot", "targetLabel": "type"},
		},
		"metricRelabelings": []any{
			map[string]any{"action": "keep", "regex": "^(container_cpu_usage_seconds_total)$", "sourceLabels": []any{"__name__"}},
			map[string]any{"action": "keep", "regex": "(^$|^kube-system$)", "sourceLabels": []any{"namespace"}},
			map[string]any{"action": "labeldrop", "regex": "^id$"},
		},
	}
}

func TestTenantSpec(t *testing.T) {
	source := gardenerCadvisorSpec()

	spec, err := tenantSpec(source)
	require.NoError(t, err)

	relabelings := spec["relabelings"].([]any)
	assert.Equal(t, "fundament-tenant-cadvisor", relabelings[0].(map[string]any)["replacement"])
	assert.Equal(t, "shoot", relabelings[1].(map[string]any)["replacement"], "the series stay shoot-side")

	metric := spec["metricRelabelings"].([]any)
	assert.Equal(t, "^(container_cpu_usage_seconds_total)$", metric[0].(map[string]any)["regex"], "metric selection is kept")
	assert.Equal(t, "^tnt-.+$", metric[1].(map[string]any)["regex"])
	assert.Equal(t, "HTTPS", spec["scheme"])

	assert.Equal(t, gardenerCadvisorSpec(), source, "the source is not modified")
}

func TestTenantSpec_RefusesAnUnknownShape(t *testing.T) {
	noFilter := gardenerCadvisorSpec()
	noFilter["metricRelabelings"] = []any{}
	_, err := tenantSpec(noFilter)
	require.ErrorIs(t, err, errUnknownShape)
	require.ErrorContains(t, err, "namespace filter")

	noJob := gardenerCadvisorSpec()
	delete(noJob, "relabelings")
	_, err = tenantSpec(noJob)
	require.ErrorIs(t, err, errUnknownShape)
	require.ErrorContains(t, err, "relabelings missing")
}
