package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

// Pins rookCRDNames to the kinds the reconcilers watch: a watched CRD not
// waited on at install can fail the cache sync and take the manager down.
func TestRookCRDNamesCoverWatchedKinds(t *testing.T) {
	t.Parallel()
	for _, plural := range []string{"cephclusters", "cephblockpools", "cephfilesystems", "cephobjectstores"} {
		assert.Contains(t, rookCRDNames, plural+".ceph.rook.io")
	}
}

// Rook upgrades one minor version at a time, so the plugin installs only over
// no Rook or its own version, and refuses any other.
func TestIncompatibleRook(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		current        *helm.ReleaseStatus
		clusterExists  bool
		daemonVersions []string
		refused        bool
	}{
		{"fresh cluster", nil, false, nil, false},
		{"release at the pin", &helm.ReleaseStatus{Status: "deployed", ChartVersion: rookChartVersion}, true, nil, false},
		// The repository serves v1.21.1 as 1.21.1, which helm records.
		{"release at the pin, recorded without v", &helm.ReleaseStatus{Status: "deployed", ChartVersion: strings.TrimPrefix(rookChartVersion, "v")}, true, nil, false},
		{"release at another version", &helm.ReleaseStatus{Status: "deployed", ChartVersion: "v1.16.0"}, true, nil, true},
		{"failed release at another version", &helm.ReleaseStatus{Status: "failed", ChartVersion: "v1.16.0"}, false, nil, true},
		{"uninstalled release, no cluster", &helm.ReleaseStatus{Status: "uninstalled", ChartVersion: "v1.16.0"}, false, nil, false},
		// Rook's CRDs outlive helm uninstall, and with them the cluster.
		{"no release, cluster at another version", nil, true, []string{"v1.16.0", "v1.16.0"}, true},
		{"uninstalled release, cluster at another version", &helm.ReleaseStatus{Status: "uninstalled", ChartVersion: "v1.16.0"}, true, []string{"v1.16.0"}, true},
		{"no release, cluster at the pin", nil, true, []string{rookChartVersion, rookChartVersion}, false},
		{"no release, cluster partly at another version", nil, true, []string{rookChartVersion, "v1.20.5"}, true},
		{"no release, cluster without labelled daemons", nil, true, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := incompatibleRook(tt.current, tt.clusterExists, tt.daemonVersions)
			if !tt.refused {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ceph-rook-runbook.md")
		})
	}
}

// runningCluster reads the version from Rook's daemons in the cluster's
// namespace only: the operator Deployment is gone with the release, and other
// namespaces are another cluster's.
func TestRunningCluster(t *testing.T) {
	t.Parallel()
	daemon := func(name, namespace, version string) *appsv1.Deployment {
		d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
		if version != "" {
			d.Labels = map[string]string{rookVersionLabel: version}
		}
		return d
	}

	exists, versions, err := runningCluster(context.Background(), newFakeClient(t, daemon("rook-ceph-mon-a", testNamespace, "v1.16.0")), testNamespace)
	require.NoError(t, err)
	assert.False(t, exists, "no CephCluster")
	assert.Empty(t, versions)

	cluster := BootstrapCephCluster(testNamespace, testConfig())
	kube := newFakeClient(t, cluster,
		daemon("rook-ceph-mon-a", testNamespace, "v1.16.0"),
		daemon("rook-ceph-osd-0", testNamespace, "v1.16.0"),
		daemon("unrelated", testNamespace, ""),
		daemon("rook-ceph-mon-a", "other", "v1.21.1"),
	)
	exists, versions, err = runningCluster(context.Background(), kube, testNamespace)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.ElementsMatch(t, []string{"v1.16.0", "v1.16.0"}, versions)
}
