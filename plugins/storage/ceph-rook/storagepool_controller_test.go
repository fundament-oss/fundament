package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

const testNamespace = "rook-ceph"

// testScheme mirrors buildScheme and additionally registers the Rook kinds as
// unstructured list types, which the fake client needs in order to track them.
func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, apiextensionsv1.AddToScheme(s))
	require.NoError(t, v1alpha1.AddToScheme(s))
	for _, kind := range []string{"CephCluster", "CephBlockPool"} {
		gvk := schema.GroupVersionKind{Group: "ceph.rook.io", Version: "v1", Kind: kind}
		s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		listGVK := gvk
		listGVK.Kind = kind + "List"
		s.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})
	}
	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.StoragePool{}, &v1alpha1.Disk{}).
		Build()
}

func newReconciler(c client.Client, s *runtime.Scheme) *StoragePoolReconciler {
	return &StoragePoolReconciler{Client: c, ClusterNamespace: testNamespace, Scheme: s}
}

func testDisk(name, node, path string, size int64, available bool) *v1alpha1.Disk {
	return &v1alpha1.Disk{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1alpha1.DiskStatus{
			Node: node, Path: path, SizeBytes: size,
			Type: v1alpha1.DiskTypeSSD, Available: available,
		},
	}
}

func testPool(name string, created time.Time, disks ...string) *v1alpha1.StoragePool {
	p := poolAt(name, created, disks...)
	p.Spec.Replication = "auto"
	return &p
}

func cephCluster() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephCluster")
	u.SetName(cephClusterName)
	u.SetNamespace(testNamespace)
	u.Object["spec"] = map[string]any{"storage": map[string]any{"nodes": []any{}}}
	return u
}

func reconcilePool(t *testing.T, r *StoragePoolReconciler, name string) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name},
	})
}

func getPool(t *testing.T, c client.Client, name string) *v1alpha1.StoragePool {
	t.Helper()
	var pool v1alpha1.StoragePool
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: name}, &pool))
	return &pool
}

// cephClusterDevices returns the device paths configured per node.
func cephClusterDevices(t *testing.T, c client.Client) map[string][]string {
	t.Helper()
	cc := &unstructured.Unstructured{}
	cc.SetAPIVersion(cephAPIVersion)
	cc.SetKind("CephCluster")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: cephClusterName}, cc))

	nodes, _, err := unstructured.NestedSlice(cc.Object, "spec", "storage", "nodes")
	require.NoError(t, err)

	out := map[string][]string{}
	for _, n := range nodes {
		node := n.(map[string]any)
		name := node["name"].(string)
		devices, _ := node["devices"].([]any)
		for _, d := range devices {
			out[name] = append(out[name], d.(map[string]any)["name"].(string))
		}
	}
	return out
}

// The happy path: a pool over two nodes produces a prefixed StorageClass and
// CephBlockPool, folds its disks into the CephCluster, and reports its own
// selection back in status.
func TestReconcileCreatesDerivedObjects(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	now := time.Now()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testDisk("node-b-1", "node-b", "/dev/sdb", 200, true),
		testPool("pool", now, "node-a-1", "node-b-1"),
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "ceph-pool"}, &sc))
	assert.Equal(t, "rook-ceph.rbd.csi.ceph.com", sc.Provisioner)
	assert.Equal(t, "ceph-pool", sc.Parameters["pool"])
	require.Len(t, sc.OwnerReferences, 1)
	assert.Equal(t, "pool", sc.OwnerReferences[0].Name)
	assert.True(t, *sc.OwnerReferences[0].Controller, "cascade delete needs a controller ref")

	cbp := &unstructured.Unstructured{}
	cbp.SetAPIVersion(cephAPIVersion)
	cbp.SetKind("CephBlockPool")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "ceph-pool"}, cbp))

	assert.Equal(t, map[string][]string{
		"node-a": {"/dev/sdb"},
		"node-b": {"/dev/sdb"},
	}, cephClusterDevices(t, c))

	pool := getPool(t, c, "pool")
	assert.Equal(t, v1alpha1.PhaseProvisioning, pool.Status.Phase)
	assert.Equal(t, "ceph-pool", pool.Status.StorageClassName)
	assert.Equal(t, 2, pool.Status.SelectedDiskCount)
	assert.Equal(t, int64(300), pool.Status.RawCapacityBytes)
	assert.Equal(t, 2, pool.Status.Replicas)
	assert.Equal(t, "host", pool.Status.FailureDomain)
}

// A pre-existing StorageClass the pool does not own must never be adopted:
// adopting it would also mean deleting it when the pool goes away.
func TestReconcileRefusesToAdoptForeignStorageClass(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	foreign := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "ceph-pool"},
		Provisioner: "rancher.io/local-path",
	}
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		foreign,
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "pool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by this StoragePool")

	// The foreign object is untouched, and the operator is told why.
	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "ceph-pool"}, &sc))
	assert.Equal(t, "rancher.io/local-path", sc.Provisioner)
	assert.Empty(t, sc.OwnerReferences)

	pool := getPool(t, c, "pool")
	assert.Equal(t, v1alpha1.PhaseDegraded, pool.Status.Phase)
	assert.Contains(t, pool.Status.Message, "not owned by this StoragePool")
}

// Same rule for Rook's own CephBlockPools, which live in the same namespace.
func TestReconcileRefusesToAdoptForeignBlockPool(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	foreign := &unstructured.Unstructured{}
	foreign.SetAPIVersion(cephAPIVersion)
	foreign.SetKind("CephBlockPool")
	foreign.SetName("ceph-pool")
	foreign.SetNamespace(testNamespace)

	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		foreign,
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "pool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CephBlockPool")
	assert.Contains(t, err.Error(), "not owned by this StoragePool")
}

// A disk listed by two pools belongs to the older one; the younger pool skips
// it rather than double-counting the same device.
func TestReconcileSkipsDisksClaimedByAnotherPool(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	early := time.Now().Add(-time.Hour)
	c := newFakeClient(t,
		cephCluster(),
		testDisk("shared", "node-a", "/dev/sdb", 100, true),
		testDisk("own", "node-a", "/dev/sdc", 50, true),
		testPool("older", early, "shared"),
		testPool("newer", early.Add(time.Minute), "shared", "own"),
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "newer")
	require.NoError(t, err)

	pool := getPool(t, c, "newer")
	assert.Equal(t, 1, pool.Status.SelectedDiskCount, "the contested disk is not counted")
	assert.Equal(t, int64(50), pool.Status.RawCapacityBytes)
	assert.Contains(t, pool.Status.Message, "claimed by older")

	// The union still carries the contested disk, because the older pool owns it.
	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb", "/dev/sdc"}},
		cephClusterDevices(t, c))
}

// Once Ceph consumes a device it stops reporting as empty. Dropping unavailable
// disks would therefore pull live OSDs out of the CephCluster on every
// subsequent reconcile.
func TestReconcileKeepsUnavailableDisksInUse(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	c := newFakeClient(t,
		cephCluster(),
		testDisk("in-use", "node-a", "/dev/sdb", 100, false),
		testPool("pool", time.Now(), "in-use"),
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c))
	assert.Equal(t, 1, getPool(t, c, "pool").Status.SelectedDiskCount)
}

func TestReconcileReportsMissingDisks(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	c := newFakeClient(t,
		cephCluster(),
		testDisk("present", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "present", "vanished"),
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	pool := getPool(t, c, "pool")
	assert.Equal(t, 1, pool.Status.SelectedDiskCount)
	assert.Contains(t, pool.Status.Message, "skipped missing disks: vanished")
}

// Deleting a pool has to shrink the CephCluster's device list. Nothing else
// would: the CephCluster carries no owner reference, and the surviving pool
// gets no event of its own.
func TestReconcileRecomputesUnionAfterPoolDeletion(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	now := time.Now()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testDisk("b", "node-b", "/dev/sdb", 100, true),
		testPool("keep", now, "a"),
		testPool("drop", now, "b"),
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "keep")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"node-a": {"/dev/sdb"},
		"node-b": {"/dev/sdb"},
	}, cephClusterDevices(t, c))

	require.NoError(t, c.Delete(context.Background(), testPool("drop", now, "b")))

	// The delete event carries the deleted pool's own key.
	_, err = reconcilePool(t, r, "drop")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c),
		"the deleted pool's disks must leave the CephCluster")
}

// A pool that is terminating has already released its disks.
func TestReconcileDropsTerminatingPoolFromUnion(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	now := time.Now()

	terminating := testPool("going", now, "b")
	terminating.Finalizers = []string{"test/keep-visible"}

	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testDisk("b", "node-b", "/dev/sdb", 100, true),
		testPool("keep", now, "a"),
		terminating,
	)
	r := newReconciler(c, s)

	require.NoError(t, c.Delete(context.Background(), terminating))

	_, err := reconcilePool(t, r, "keep")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c))
}

// The same physical device reachable through two pools must appear once.
func TestReconcileDeduplicatesSharedDisksInUnion(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	now := time.Now()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("shared", "node-a", "/dev/sdb", 100, true),
		testPool("first", now, "shared"),
		testPool("second", now.Add(time.Minute), "shared"),
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "first")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c))
}

// A Ready CephBlockPool stops the 30s provisioning requeue.
func TestReconcileReadyWhenBlockPoolReady(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c, s)

	res, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)
	assert.Equal(t, provisioningRequeue, res.RequeueAfter)
	assert.Equal(t, v1alpha1.PhaseProvisioning, getPool(t, c, "pool").Status.Phase)

	cbp := &unstructured.Unstructured{}
	cbp.SetAPIVersion(cephAPIVersion)
	cbp.SetKind("CephBlockPool")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "ceph-pool"}, cbp))
	require.NoError(t, unstructured.SetNestedField(cbp.Object, "Ready", "status", "phase"))
	require.NoError(t, c.Update(context.Background(), cbp))

	res, err = reconcilePool(t, r, "pool")
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter, "a Ready pool does not need re-checking")
	assert.Equal(t, v1alpha1.PhaseReady, getPool(t, c, "pool").Status.Phase)
}

// Reconciling twice must be a no-op, not a rejected update: every field the
// StorageClass renders is immutable, so a second pass that tried to rewrite
// them would fail forever.
func TestReconcileIsIdempotent(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c, s)

	for i := range 3 {
		_, err := reconcilePool(t, r, "pool")
		require.NoErrorf(t, err, "reconcile %d", i)
	}

	pool := getPool(t, c, "pool")
	assert.Equal(t, v1alpha1.PhaseProvisioning, pool.Status.Phase)
	assert.Empty(t, pool.Status.Message)
}

func TestImmutableStorageClassDrift(t *testing.T) {
	t.Parallel()
	desired := RenderStorageClass("ceph-pool", testNamespace, "ceph-pool")

	assert.Empty(t, immutableStorageClassDrift(desired.DeepCopy(), desired))

	changed := desired.DeepCopy()
	changed.Provisioner = "other.csi.example.com"
	changed.Parameters = map[string]string{"pool": "different"}
	assert.Equal(t, []string{"parameters", "provisioner"}, immutableStorageClassDrift(changed, desired))

	// allowVolumeExpansion is the one field Kubernetes lets us update.
	expandable := desired.DeepCopy()
	expandable.AllowVolumeExpansion = ptr(false)
	assert.Empty(t, immutableStorageClassDrift(expandable, desired))
}

// Drift on an immutable field must be reported, not retried into a permanent
// API rejection.
func TestReconcileReportsImmutableStorageClassDrift(t *testing.T) {
	t.Parallel()
	s := testScheme(t)
	pool := testPool("pool", time.Now(), "a")

	yes := true
	stale := RenderStorageClass("ceph-pool", "a-different-namespace", "ceph-pool")
	stale.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: v1alpha1.GroupVersion.String(), Kind: "StoragePool",
		Name: pool.Name, UID: pool.UID, Controller: &yes,
	}}

	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		pool,
		stale,
	)
	r := newReconciler(c, s)

	_, err := reconcilePool(t, r, "pool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable field(s) parameters")
	assert.Contains(t, err.Error(), "Delete the StorageClass")
	assert.Equal(t, v1alpha1.PhaseDegraded, getPool(t, c, "pool").Status.Phase)
}
