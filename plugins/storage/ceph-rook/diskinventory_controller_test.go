package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

func TestNodeFromConfigMap(t *testing.T) {
	tests := []struct {
		name   string
		cmName string
		labels map[string]string
		want   string
	}{
		{
			name:   "label takes precedence",
			cmName: "local-device-worker-1",
			labels: map[string]string{"rook.io/node": "worker-1"},
			want:   "worker-1",
		},
		{
			name:   "falls back to stripping local-device- prefix",
			cmName: "local-device-worker-2",
			labels: map[string]string{"app": "rook-discover"},
			want:   "worker-2",
		},
		{
			name:   "no prefix to strip returns name as-is",
			cmName: "some-other-cm",
			labels: map[string]string{},
			want:   "some-other-cm",
		},
		{
			name:   "empty node label falls back to name stripping",
			cmName: "local-device-node3",
			labels: map[string]string{"rook.io/node": ""},
			want:   "node3",
		},
		{
			name:   "nil labels falls back to name stripping",
			cmName: "local-device-node4",
			labels: nil,
			want:   "node4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nodeFromConfigMap(tc.cmName, tc.labels)
			assert.Equal(t, tc.want, got)
		})
	}
}

// discoveryConfigMap builds what rook-discover writes per node.
func discoveryConfigMap(t *testing.T, node string, devices ...rawDevice) *corev1.ConfigMap {
	t.Helper()
	raw, err := json.Marshal(devices)
	require.NoError(t, err)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-device-" + node,
			Namespace: testNamespace,
			Labels:    map[string]string{"app": discoverAppLabel, "rook.io/node": node},
		},
		Data: map[string]string{"devices": string(raw)},
	}
}

func reconcileDiscovery(t *testing.T, r *DiskInventoryReconciler, node string) error {
	t.Helper()
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: "local-device-" + node},
	})
	return err
}

func getDisk(t *testing.T, c client.Client, name string) *v1alpha1.Disk {
	t.Helper()
	var disk v1alpha1.Disk
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: name}, &disk))
	return &disk
}

// The picker filters on claimedBy, so it must reflect a pool created a moment
// ago rather than wait out rook-discover's 60m sweep.
func TestDiskInventorySetsClaimedByFromPools(t *testing.T) {
	t.Parallel()
	node := "node-a"
	cm := discoveryConfigMap(t, node, rawDevice{Name: "sdb", Size: 100, Type: "disk", Empty: true})
	diskName := DiskName(node, "path:/dev/sdb")

	c := newFakeClient(t, cm)
	r := &DiskInventoryReconciler{Client: c, RookNamespace: testNamespace}

	require.NoError(t, reconcileDiscovery(t, r, node))
	disk := getDisk(t, c, diskName)
	assert.True(t, disk.Status.Available)
	assert.Empty(t, disk.Status.ClaimedBy, "no pool exists yet")

	// A pool takes the disk; the next reconcile must reflect the claim.
	require.NoError(t, c.Create(context.Background(), testPool("pool", time.Now(), diskName)))
	require.NoError(t, reconcileDiscovery(t, r, node))
	assert.Equal(t, "pool", getDisk(t, c, diskName).Status.ClaimedBy)
}

// A pool event has to reach every node's ConfigMap.
func TestPoolToDiscoveryConfigMapsCoversEveryNode(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		discoveryConfigMap(t, "node-a", rawDevice{Name: "sdb", Type: "disk", Empty: true}),
		discoveryConfigMap(t, "node-b", rawDevice{Name: "sdb", Type: "disk", Empty: true}),
		// Not a discovery ConfigMap: must not be enqueued.
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: testNamespace}},
	)
	r := &DiskInventoryReconciler{Client: c, RookNamespace: testNamespace}

	reqs := r.poolToDiscoveryConfigMaps(context.Background(), testPool("pool", time.Now()))

	names := make([]string, 0, len(reqs))
	for _, req := range reqs {
		names = append(names, req.Name)
	}
	assert.ElementsMatch(t, []string{"local-device-node-a", "local-device-node-b"}, names)
}

// Marked unavailable, never deleted: repo policy is soft deletes only.
func TestDiskInventorySoftDeletesVanishedDisks(t *testing.T) {
	t.Parallel()
	node := "node-a"
	diskName := DiskName(node, "path:/dev/sdb")

	c := newFakeClient(t, discoveryConfigMap(t, node,
		rawDevice{Name: "sdb", Size: 100, Type: "disk", Empty: true}))
	r := &DiskInventoryReconciler{Client: c, RookNamespace: testNamespace}

	require.NoError(t, reconcileDiscovery(t, r, node))
	require.True(t, getDisk(t, c, diskName).Status.Available)

	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "local-device-" + node}, &cm))
	cm.Data["devices"] = "[]"
	require.NoError(t, c.Update(context.Background(), &cm))

	require.NoError(t, reconcileDiscovery(t, r, node))
	disk := getDisk(t, c, diskName)
	assert.False(t, disk.Status.Available, "vanished disks are marked unavailable")
}

// A departed node takes its ConfigMap with it, leaving no device list to diff.
// Returning early on NotFound left those Disks available=true forever.
func TestDiskInventorySoftDeletesDisksOfDepartedNode(t *testing.T) {
	t.Parallel()
	node := "node-a"
	diskName := DiskName(node, "path:/dev/sdb")

	c := newFakeClient(t, discoveryConfigMap(t, node,
		rawDevice{Name: "sdb", Size: 100, Type: "disk", Empty: true}))
	r := &DiskInventoryReconciler{Client: c, RookNamespace: testNamespace}

	require.NoError(t, reconcileDiscovery(t, r, node))
	require.True(t, getDisk(t, c, diskName).Status.Available)

	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "local-device-" + node}, &cm))
	require.NoError(t, c.Delete(context.Background(), &cm))

	require.NoError(t, reconcileDiscovery(t, r, node))
	assert.False(t, getDisk(t, c, diskName).Status.Available,
		"disks on a departed node must stop being offered")
}

// The node comes from the request name once the labels are gone; other nodes'
// disks must be left alone.
func TestDiskInventoryDepartedNodeLeavesOtherNodesAlone(t *testing.T) {
	t.Parallel()
	gone, staying := "node-a", "node-b"
	goneDisk := DiskName(gone, "path:/dev/sdb")
	stayingDisk := DiskName(staying, "path:/dev/sdb")

	c := newFakeClient(t,
		discoveryConfigMap(t, gone, rawDevice{Name: "sdb", Size: 100, Type: "disk", Empty: true}),
		discoveryConfigMap(t, staying, rawDevice{Name: "sdb", Size: 100, Type: "disk", Empty: true}),
	)
	r := &DiskInventoryReconciler{Client: c, RookNamespace: testNamespace}
	require.NoError(t, reconcileDiscovery(t, r, gone))
	require.NoError(t, reconcileDiscovery(t, r, staying))

	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "local-device-" + gone}, &cm))
	require.NoError(t, c.Delete(context.Background(), &cm))
	require.NoError(t, reconcileDiscovery(t, r, gone))

	assert.False(t, getDisk(t, c, goneDisk).Status.Available)
	assert.True(t, getDisk(t, c, stayingDisk).Status.Available, "node-b never went anywhere")
}

// A Disk CR is named after its stable identity, so a kernel rename must not
// produce a second CR for the same physical device.
func TestDiskInventoryKeepsDiskNameAcrossKernelRename(t *testing.T) {
	t.Parallel()
	node := "node-a"

	c := newFakeClient(t, discoveryConfigMap(t, node,
		rawDevice{Name: "sdb", Size: 100, Type: "disk", Empty: true, WWN: "0xABC",
			DevLinks: "/dev/disk/by-id/wwn-0xABC"}))
	r := &DiskInventoryReconciler{Client: c, RookNamespace: testNamespace}
	require.NoError(t, reconcileDiscovery(t, r, node))

	var before v1alpha1.DiskList
	require.NoError(t, c.List(context.Background(), &before))
	require.Len(t, before.Items, 1)

	// Reboot: the same device comes back as /dev/sdc.
	raw, err := json.Marshal([]rawDevice{{
		Name: "sdc", Size: 100, Type: "disk", Empty: true, WWN: "0xABC",
		DevLinks: "/dev/disk/by-id/wwn-0xABC",
	}})
	require.NoError(t, err)
	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "local-device-" + node}, &cm))
	cm.Data["devices"] = string(raw)
	require.NoError(t, c.Update(context.Background(), &cm))
	require.NoError(t, reconcileDiscovery(t, r, node))

	var after v1alpha1.DiskList
	require.NoError(t, c.List(context.Background(), &after))
	require.Len(t, after.Items, 1, "a rename must not fork the inventory")
	assert.Equal(t, before.Items[0].Name, after.Items[0].Name)
	assert.True(t, after.Items[0].Status.Available, "and must not soft-delete the survivor")
	assert.Equal(t, "/dev/sdc", after.Items[0].Status.Path, "the kernel path still tracks reality")
}

// The watch predicate filters events, not the cache. The scope must match what
// DiskInventoryReconciler queries, or the cached client returns nothing.
// configMapCacheScope finds the ConfigMap entry in a ByObject map. It cannot
// index the map: the keys are interface values holding pointers, so a fresh
// &corev1.ConfigMap{} misses. controller-runtime resolves them by GVK, so
// matching the concrete type mirrors that.
func configMapCacheScope(t *testing.T, opts cache.Options) (cache.ByObject, bool) {
	t.Helper()
	for obj, byObject := range opts.ByObject {
		if _, ok := obj.(*corev1.ConfigMap); ok {
			return byObject, true
		}
	}
	return cache.ByObject{}, false
}

func TestCacheOptionsScopeConfigMapsToDiscovery(t *testing.T) {
	t.Parallel()
	cfg := Config{RookNamespace: "rook-ceph"}

	byObject, ok := configMapCacheScope(t, cacheOptions(cfg))
	require.True(t, ok, "ConfigMaps must be scoped; the default caches the whole cluster")

	nsCfg, ok := byObject.Namespaces[cfg.RookNamespace]
	require.True(t, ok, "scoped to the namespace the discovery daemon writes to")
	require.NotNil(t, nsCfg.LabelSelector)

	assert.True(t, nsCfg.LabelSelector.Matches(labels.Set{"app": discoverAppLabel}),
		"the selector must admit the ConfigMaps the reconciler reads")
	assert.False(t, nsCfg.LabelSelector.Matches(labels.Set{"app": "something-else"}))
}

// A non-default namespace must follow, or the plugin caches nothing.
func TestCacheOptionsFollowsRookNamespace(t *testing.T) {
	t.Parallel()
	byObject, found := configMapCacheScope(t, cacheOptions(Config{RookNamespace: "ceph-operator"}))
	require.True(t, found)

	_, ok := byObject.Namespaces["ceph-operator"]
	assert.True(t, ok)
	_, ok = byObject.Namespaces["rook-ceph"]
	assert.False(t, ok, "the default namespace must not be cached when it is not configured")
}
