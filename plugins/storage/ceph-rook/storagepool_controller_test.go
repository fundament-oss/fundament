package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

const testNamespace = "rook-ceph"

// testScheme mirrors buildScheme, plus the Rook kinds as unstructured list types
// so the fake client can track them.
func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, apiextensionsv1.AddToScheme(s))
	require.NoError(t, v1alpha1.AddToScheme(s))
	for _, kind := range []string{"CephCluster", "CephBlockPool", "CephFilesystem"} {
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
		WithStatusSubresource(&v1alpha1.StoragePool{}, &v1alpha1.Disk{}, &v1alpha1.BlockStorage{}, &v1alpha1.FileStorage{}).
		Build()
}

func newReconciler(c client.Client) *StoragePoolReconciler {
	return &StoragePoolReconciler{Client: c, ClusterNamespace: testNamespace}
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

// Happy path: disks folded into the CephCluster, and the pool's own selection
// reported in status. StoragePoolReconciler derives no StorageClass or
// CephBlockPool of its own -- a consumer kind does that over the shared OSDs.
func TestReconcileContributesDisks(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testDisk("node-b-1", "node-b", "/dev/sdb", 200, true),
		testPool("pool", now, "node-a-1", "node-b-1"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{
		"node-a": {"/dev/sdb"},
		"node-b": {"/dev/sdb"},
	}, cephClusterDevices(t, c))

	pool := getPool(t, c, "pool")
	assert.Equal(t, v1alpha1.PhaseReady, pool.Status.Phase)
	assert.Equal(t, 2, pool.Status.SelectedDiskCount)
	assert.Equal(t, int64(300), pool.Status.RawCapacityBytes)
}

// Ready means "this pool's disks are recorded in the shared Ceph cluster".
// Without the singleton CephCluster nothing was recorded, so the pool must not
// report Ready.
func TestReconcileDegradedWithoutCephCluster(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	pool := getPool(t, c, "pool")
	assert.Equal(t, v1alpha1.PhaseDegraded, pool.Status.Phase)
	assert.Contains(t, pool.Status.Message, "CephCluster")
	assert.Equal(t, 1, pool.Status.SelectedDiskCount,
		"selection resolved; only the contribution is pending")
	assert.Zero(t, pool.Status.RawCapacityBytes, "nothing was contributed yet")
	cond := meta.FindStatusCondition(pool.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha1.ReasonCephClusterMissing, cond.Reason)
}

// When the CephCluster appears later (its watch enqueues every pool), the next
// reconcile records the disks and flips the pool to Ready.
func TestReconcileRecoversWhenCephClusterAppears(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)
	require.Equal(t, v1alpha1.PhaseDegraded, getPool(t, c, "pool").Status.Phase)

	require.NoError(t, c.Create(context.Background(), cephCluster()))

	_, err = reconcilePool(t, r, "pool")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c))
	assert.Equal(t, v1alpha1.PhaseReady, getPool(t, c, "pool").Status.Phase)
}

// A disk listed by two pools belongs to the older one; the younger pool skips
// it rather than double-counting the same device.
func TestReconcileSkipsDisksClaimedByAnotherPool(t *testing.T) {
	t.Parallel()
	early := time.Now().Add(-time.Hour)
	c := newFakeClient(t,
		cephCluster(),
		testDisk("shared", "node-a", "/dev/sdb", 100, true),
		testDisk("own", "node-a", "/dev/sdc", 50, true),
		testPool("older", early, "shared"),
		testPool("newer", early.Add(time.Minute), "shared", "own"),
	)
	r := newReconciler(c)

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
	c := newFakeClient(t,
		cephCluster(),
		testDisk("in-use", "node-a", "/dev/sdb", 100, false),
		testPool("pool", time.Now(), "in-use"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c))
	assert.Equal(t, 1, getPool(t, c, "pool").Status.SelectedDiskCount)
}

func TestReconcileReportsMissingDisks(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("present", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "present", "vanished"),
	)
	r := newReconciler(c)

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
	now := time.Now()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testDisk("b", "node-b", "/dev/sdb", 100, true),
		testPool("keep", now, "a"),
		testPool("drop", now, "b"),
	)
	r := newReconciler(c)

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
	r := newReconciler(c)

	require.NoError(t, c.Delete(context.Background(), terminating))

	_, err := reconcilePool(t, r, "keep")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c))
}

// The same physical device reachable through two pools must appear once.
func TestReconcileDeduplicatesSharedDisksInUnion(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("shared", "node-a", "/dev/sdb", 100, true),
		testPool("first", now, "shared"),
		testPool("second", now.Add(time.Minute), "shared"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "first")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c))
}

// Reconciling twice must be a no-op, not a rejected update: writeStatus's
// DeepEqual guard must recognise an unchanged status.
func TestReconcileIsIdempotent(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c)

	for i := range 3 {
		_, err := reconcilePool(t, r, "pool")
		require.NoErrorf(t, err, "reconcile %d", i)
	}

	pool := getPool(t, c, "pool")
	assert.Equal(t, v1alpha1.PhaseReady, pool.Status.Phase)
	assert.Empty(t, pool.Status.Message)
}

// A disk repeated in spec.disks must not be counted twice: selectedDiskCount
// and rawCapacityBytes are the numbers an operator sizes workloads against.
// The CRD marks the field as a set, so this only bites an object written before
// that marker existed -- but it bites silently.
func TestReconcileDeduplicatesRepeatedDisksInSpec(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a", "a", "a"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	pool := getPool(t, c, "pool")
	assert.Equal(t, 1, pool.Status.SelectedDiskCount)
	assert.Equal(t, int64(100), pool.Status.RawCapacityBytes)
	assert.Equal(t, map[string][]string{"node-a": {"/dev/sdb"}}, cephClusterDevices(t, c))
}

// Two Disk CRs that resolve to the same physical device must collapse to one
// entry, or Rook would be handed the same device twice.
func TestReconcileDeduplicatesUnionByStablePath(t *testing.T) {
	t.Parallel()

	byID := func(name, node, kernelPath, stable string) *v1alpha1.Disk {
		d := testDisk(name, node, kernelPath, 100, true)
		d.Status.StablePath = stable
		return d
	}

	c := newFakeClient(t,
		cephCluster(),
		// The same device, discovered before and after a kernel rename.
		byID("a", "node-a", "/dev/sdb", "/dev/disk/by-id/wwn-0xABC"),
		byID("b", "node-a", "/dev/sdc", "/dev/disk/by-id/wwn-0xABC"),
		testPool("pool", time.Now(), "a", "b"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"node-a": {"/dev/disk/by-id/wwn-0xABC"}},
		cephClusterDevices(t, c))
}

// The CephCluster must be pointed at the by-id path when the node reports one:
// a kernel rename would otherwise take the OSD's device out of the cluster.
func TestReconcileWritesStablePathsIntoCephCluster(t *testing.T) {
	t.Parallel()
	stable := testDisk("a", "node-a", "/dev/sdb", 100, true)
	stable.Status.StablePath = "/dev/disk/by-id/wwn-0xABC"

	c := newFakeClient(t,
		cephCluster(),
		stable,
		testDisk("b", "node-a", "/dev/loop0p1", 100, true), // no stable path
		testPool("pool", time.Now(), "a", "b"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{
		"node-a": {"/dev/disk/by-id/wwn-0xABC", "/dev/loop0p1"},
	}, cephClusterDevices(t, c))
}

// Every Disk event fans out to every pool, with identical status almost every
// time. Rewriting would bump resourceVersion and wake every watcher.
func TestReconcileDoesNotRewriteUnchangedStatus(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.NoError(t, err)
	settled := getPool(t, c, "pool").ResourceVersion

	for range 3 {
		_, err := reconcilePool(t, r, "pool")
		require.NoError(t, err)
	}
	assert.Equal(t, settled, getPool(t, c, "pool").ResourceVersion,
		"a reconcile that changes nothing must not write")
}

// A pool over zero disks used to sit in Provisioning forever behind a
// CephBlockPool with no OSDs and a StorageClass that left every PVC Pending.
// A missing disk resolves the same way (selected count zero): reported in
// status, never as a reconcile error that would strand the pool on a blip.
func TestReconcileEmptyPoolIsDegradedAndCreatesNothing(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t, cephCluster(), testPool("pool", time.Now()))

	_, err := reconcilePool(t, newReconciler(c), "pool")
	require.NoError(t, err, "an empty pool is an operator problem, not a reconcile failure")

	status := getPool(t, c, "pool").Status
	assert.Equal(t, v1alpha1.PhaseDegraded, status.Phase)
	assert.Contains(t, status.Message, "add disks to spec.disks",
		"an empty spec.disks produces no skip notes, so the message must be explicit")
	assert.Zero(t, status.SelectedDiskCount)

	var sc storagev1.StorageClass
	err = c.Get(context.Background(), types.NamespacedName{Name: DerivedName("pool")}, &sc)
	assert.True(t, apierrors.IsNotFound(err), "no StorageClass for a pool with no disks")

	cbp := &unstructured.Unstructured{}
	cbp.SetAPIVersion(cephAPIVersion)
	cbp.SetKind("CephBlockPool")
	err = c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: DerivedName("pool")}, cbp)
	assert.True(t, apierrors.IsNotFound(err), "no CephBlockPool for a pool with no disks")
}

// Same dead end as naming none, but the failed names have to be reported.
func TestReconcileEmptyPoolNamesTheMissingDisks(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t, cephCluster(), testPool("pool", time.Now(), "ghost"))

	_, err := reconcilePool(t, newReconciler(c), "pool")
	require.NoError(t, err)

	status := getPool(t, c, "pool").Status
	assert.Equal(t, v1alpha1.PhaseDegraded, status.Phase)
	assert.Contains(t, status.Message, "ghost")
}

// The previous Ready status must not be left standing.
func TestReconcileEmptyingALivePoolDegradesIt(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c)
	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))
	require.NotEqual(t, v1alpha1.PhaseDegraded, getPool(t, c, "pool").Status.Phase)

	pool := getPool(t, c, "pool")
	pool.Spec.Disks = nil
	require.NoError(t, c.Update(context.Background(), pool))

	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))
	assert.Equal(t, v1alpha1.PhaseDegraded, getPool(t, c, "pool").Status.Phase)
	assert.Empty(t, cephClusterDevices(t, c), "the pool's disks leave the CephCluster too")
}

// Every Disk event fans out to every pool, each of which rewrites the
// CephCluster. An identical write still costs a round-trip.
func TestReconcileDoesNotRewriteUnchangedCephCluster(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c)
	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))

	cc := &unstructured.Unstructured{}
	cc.SetAPIVersion(cephAPIVersion)
	cc.SetKind("CephCluster")
	key := types.NamespacedName{Namespace: testNamespace, Name: cephClusterName}
	require.NoError(t, c.Get(context.Background(), key, cc))
	settled := cc.GetResourceVersion()

	for range 3 {
		require.NoError(t, firstErr(reconcilePool(t, r, "pool")))
	}
	require.NoError(t, c.Get(context.Background(), key, cc))
	assert.Equal(t, settled, cc.GetResourceVersion())
}

func firstErr(_ ctrl.Result, err error) error { return err }

// Owns() and Watches() resolve a watched type's GVK through apiutil at builder
// time, which is the one thing about an unstructured stub that can fail — and
// it would fail at manager start, not here, so it is worth pinning for every
// kind the controllers register.
func TestRookStubsResolveGVK(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"CephCluster", "CephBlockPool", "CephFilesystem"} {
		gvk, err := apiutil.GVKForObject(rookStub(kind), testScheme(t))
		require.NoError(t, err)
		assert.Equal(t, schema.GroupVersionKind{Group: "ceph.rook.io", Version: "v1", Kind: kind}, gvk)
	}
}

// readyCondition returns the pool's Ready condition, failing if it is absent.
func readyCondition(t *testing.T, pool *v1alpha1.StoragePool) metav1.Condition {
	t.Helper()
	cond := meta.FindStatusCondition(pool.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond, "StoragePool %s has no %s condition", pool.Name, v1alpha1.ConditionReady)
	return *cond
}

// phase alone cannot say whether the controller has seen the current spec.
// `kubectl wait --for=condition=Ready` and a Flux/Argo health check read this.
// Unlike the old CephBlockPool-backed pipeline, a pool with usable disks goes
// Ready on its very first reconcile: there is no derived object left to wait on.
func TestReconcileTracksReadyCondition(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c)

	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))
	cond := readyCondition(t, getPool(t, c, "pool"))
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, v1alpha1.ReasonReady, cond.Reason)
}

// A status reporting Ready from a spec two generations old is worse than no
// status: observedGeneration is what separates current from stale.
func TestReconcileRecordsObservedGeneration(t *testing.T) {
	t.Parallel()
	// The fake client does not maintain metadata.generation, so the test sets it
	// the way the API server would on a spec write.
	pool := testPool("pool", time.Now(), "a")
	pool.Generation = 3
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testDisk("b", "node-b", "/dev/sdb", 100, true),
		pool,
	)
	r := newReconciler(c)
	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))

	assert.EqualValues(t, 3, getPool(t, c, "pool").Status.ObservedGeneration)
	assert.EqualValues(t, 3, readyCondition(t, getPool(t, c, "pool")).ObservedGeneration)

	live := getPool(t, c, "pool")
	live.Spec.Disks = []string{"a", "b"}
	live.Generation = 4
	require.NoError(t, c.Update(context.Background(), live))
	// Status still describes generation 3: this is the stale window the field exists to expose.
	assert.EqualValues(t, 3, getPool(t, c, "pool").Status.ObservedGeneration)

	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))
	assert.EqualValues(t, 4, getPool(t, c, "pool").Status.ObservedGeneration)
	assert.EqualValues(t, 4, readyCondition(t, getPool(t, c, "pool")).ObservedGeneration)
}

// The one condition an operator acts on: there is nothing to build a pool from.
func TestReconcileEmptyPoolConditionNamesTheCause(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t, cephCluster(), testPool("pool", time.Now()))
	r := newReconciler(c)
	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))

	cond := readyCondition(t, getPool(t, c, "pool"))
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha1.ReasonNoUsableDisks, cond.Reason)
	assert.Contains(t, cond.Message, "add disks to spec.disks")
}

// setDegraded runs on the error path, where the reconcile returns before
// building a status. The condition still has to carry the cause. Adoption
// refusal moved out with applyOwnedRookObject/applyOwnedStorageClass (Task 4), so the
// error is forced here by failing the CephCluster update instead.
func TestReconcileErrorConditionCarriesTheCause(t *testing.T) {
	t.Parallel()
	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(cephCluster(), testDisk("a", "node-a", "/dev/sdb", 100, true), testPool("pool", time.Now(), "a")).
		WithStatusSubresource(&v1alpha1.StoragePool{}, &v1alpha1.Disk{}, &v1alpha1.BlockStorage{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if obj.GetObjectKind().GroupVersionKind().Kind == "CephCluster" {
					return errors.New("boom")
				}
				return c.Update(ctx, obj, opts...)
			},
		}).
		Build()
	r := newReconciler(c)

	_, err := reconcilePool(t, r, "pool")
	require.Error(t, err)

	cond := readyCondition(t, getPool(t, c, "pool"))
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha1.ReasonReconcileError, cond.Reason)
	assert.Contains(t, cond.Message, "boom")
}

// SetStatusCondition only resets lastTransitionTime when the status flips. If it
// reset on every pass, the DeepEqual guard in writeStatus would never hold and
// every Disk event in the cluster would write every pool.
func TestReconcileKeepsLastTransitionTimeWhileReady(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c)
	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))

	first := readyCondition(t, getPool(t, c, "pool")).LastTransitionTime
	settled := getPool(t, c, "pool").ResourceVersion

	for range 3 {
		require.NoError(t, firstErr(reconcilePool(t, r, "pool")))
	}
	assert.Equal(t, first, readyCondition(t, getPool(t, c, "pool")).LastTransitionTime)
	assert.Equal(t, settled, getPool(t, c, "pool").ResourceVersion,
		"a reconcile that changes nothing must not write")
}

// The conditions handed to SetStatusCondition must not alias the refetched
// object's: it updates an existing condition in place, through a pointer into
// the backing array, so a shared slice mutates both sides and the DeepEqual
// guard ends up comparing a condition against itself. A reason- or
// message-only change would then never reach the API server.
func TestWriteStatusPersistsConditionOnlyChange(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("a", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "a"),
	)
	r := newReconciler(c)
	require.NoError(t, firstErr(reconcilePool(t, r, "pool")))

	pool := getPool(t, c, "pool")
	before := readyCondition(t, pool)
	require.Equal(t, metav1.ConditionTrue, before.Status)

	// Every observable field stays as it is; only the condition's reason and
	// message move. Status is deliberately unchanged so the write can only be
	// driven by the condition.
	status := pool.Status
	require.NoError(t, r.writeStatus(context.Background(), pool, &status, &metav1.Condition{
		Status:  metav1.ConditionTrue,
		Reason:  v1alpha1.ReasonProvisioning,
		Message: "reason and message moved, nothing else did",
	}))

	after := readyCondition(t, getPool(t, c, "pool"))
	assert.Equal(t, v1alpha1.ReasonProvisioning, after.Reason)
	assert.Equal(t, "reason and message moved, nothing else did", after.Message)
	assert.Equal(t, before.LastTransitionTime, after.LastTransitionTime,
		"the condition did not flip, so its transition time must stand")
}
