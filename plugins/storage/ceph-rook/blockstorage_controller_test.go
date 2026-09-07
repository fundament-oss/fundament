package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

func testBlockStorage(name string) *v1alpha1.BlockStorage {
	return &v1alpha1.BlockStorage{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("uid-" + name)},
		Spec:       v1alpha1.BlockStorageSpec{Replication: "auto"},
	}
}

func newBlockReconciler(c client.Client) *BlockStorageReconciler {
	return &BlockStorageReconciler{Client: c, ClusterNamespace: testNamespace, RookNamespace: testNamespace}
}

func reconcileBlock(t *testing.T, r *BlockStorageReconciler, name string) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: name}})
}

func getBlock(t *testing.T, c client.Client, name string) *v1alpha1.BlockStorage {
	t.Helper()
	var bs v1alpha1.BlockStorage
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: name}, &bs))
	return &bs
}

// Happy path: derived CephBlockPool + StorageClass, replication sized on the
// StoragePool union's node count, status populated.
func TestBlockStorageCreatesDerivedObjects(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testDisk("node-b-1", "node-b", "/dev/sdb", 200, true),
		testPool("pool", time.Now(), "node-a-1", "node-b-1"),
		testBlockStorage("fast"),
	)
	r := newBlockReconciler(c)

	res, err := reconcileBlock(t, r, "fast")
	require.NoError(t, err)
	assert.Equal(t, provisioningRequeue, res.RequeueAfter)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "ceph-fast"}, &sc))
	assert.Equal(t, "rook-ceph.rbd.csi.ceph.com", sc.Provisioner)
	assert.Equal(t, "ceph-fast", sc.Parameters["pool"])
	require.Len(t, sc.OwnerReferences, 1)
	assert.Equal(t, "BlockStorage", sc.OwnerReferences[0].Kind)
	assert.Equal(t, "fast", sc.OwnerReferences[0].Name)

	cbp := &unstructured.Unstructured{}
	cbp.SetAPIVersion(cephAPIVersion)
	cbp.SetKind("CephBlockPool")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "ceph-fast"}, cbp))
	require.Len(t, cbp.GetOwnerReferences(), 1)
	assert.Equal(t, "BlockStorage", cbp.GetOwnerReferences()[0].Kind)

	bs := getBlock(t, c, "fast")
	assert.Equal(t, v1alpha1.PhaseProvisioning, bs.Status.Phase)
	assert.Equal(t, "ceph-fast", bs.Status.StorageClassName)
	assert.Equal(t, 2, bs.Status.Replicas)
	assert.Equal(t, "host", bs.Status.FailureDomain)
}

// No StoragePool contributes disks: Degraded, nothing created.
func TestBlockStorageDegradedWithoutOSDs(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t, cephCluster(), testBlockStorage("fast"))
	r := newBlockReconciler(c)

	_, err := reconcileBlock(t, r, "fast")
	require.NoError(t, err)

	var sc storagev1.StorageClass
	getErr := c.Get(context.Background(), types.NamespacedName{Name: "ceph-fast"}, &sc)
	assert.True(t, apierrors.IsNotFound(getErr), "no StorageClass without OSDs")

	bs := getBlock(t, c, "fast")
	assert.Equal(t, v1alpha1.PhaseDegraded, bs.Status.Phase)
	assert.Contains(t, bs.Status.Message, "create a StoragePool")
	cond := meta.FindStatusCondition(bs.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha1.ReasonNoOSDs, cond.Reason)
}

// Adopting a StorageClass we do not own would delete it with the BlockStorage.
func TestBlockStorageRefusesForeignStorageClass(t *testing.T) {
	t.Parallel()
	foreign := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "ceph-fast"},
		Provisioner: "rancher.io/local-path",
	}
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testBlockStorage("fast"),
		foreign,
	)
	r := newBlockReconciler(c)

	_, err := reconcileBlock(t, r, "fast")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by this BlockStorage")
	assert.True(t, errors.Is(err, reconcile.TerminalError(nil)),
		"refusing to adopt a foreign object is not retryable")

	bs := getBlock(t, c, "fast")
	assert.Equal(t, v1alpha1.PhaseDegraded, bs.Status.Phase)
}

// Same rule for Rook's own CephBlockPools in that namespace: adopting one we do
// not own would delete it with the BlockStorage.
func TestBlockStorageRefusesForeignCephBlockPool(t *testing.T) {
	t.Parallel()
	foreign := &unstructured.Unstructured{}
	foreign.SetAPIVersion(cephAPIVersion)
	foreign.SetKind("CephBlockPool")
	foreign.SetName("ceph-fast")
	foreign.SetNamespace(testNamespace)

	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testBlockStorage("fast"),
		foreign,
	)
	r := newBlockReconciler(c)

	_, err := reconcileBlock(t, r, "fast")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CephBlockPool")
	assert.Contains(t, err.Error(), "not owned by this BlockStorage")
	assert.True(t, errors.Is(err, reconcile.TerminalError(nil)),
		"refusing to adopt a foreign object is not retryable")

	bs := getBlock(t, c, "fast")
	assert.Equal(t, v1alpha1.PhaseDegraded, bs.Status.Phase)
}

// CephBlockPool reporting Ready flips phase and condition.
func TestBlockStorageReadyWhenBlockPoolReady(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testBlockStorage("fast"),
	)
	r := newBlockReconciler(c)
	_, err := reconcileBlock(t, r, "fast")
	require.NoError(t, err)

	cbp := &unstructured.Unstructured{}
	cbp.SetAPIVersion(cephAPIVersion)
	cbp.SetKind("CephBlockPool")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "ceph-fast"}, cbp))
	require.NoError(t, unstructured.SetNestedField(cbp.Object, "Ready", "status", "phase"))
	require.NoError(t, c.Update(context.Background(), cbp))

	res, err := reconcileBlock(t, r, "fast")
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter, "Ready pool needs no requeue")

	bs := getBlock(t, c, "fast")
	assert.Equal(t, v1alpha1.PhaseReady, bs.Status.Phase)
	cond := meta.FindStatusCondition(bs.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

// Explicit replication is clamped to the contributing node count.
func TestBlockStorageClampsReplicationToNodes(t *testing.T) {
	t.Parallel()
	bs := testBlockStorage("fast")
	bs.Spec.Replication = "3"
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		bs,
	)
	r := newBlockReconciler(c)
	_, err := reconcileBlock(t, r, "fast")
	require.NoError(t, err)

	got := getBlock(t, c, "fast")
	assert.Equal(t, 1, got.Status.Replicas)
	assert.Contains(t, got.Status.Message, "clamped")
}

// Identical status must not be rewritten (Disk events fan out to every consumer).
func TestBlockStorageDoesNotRewriteUnchangedStatus(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testBlockStorage("fast"),
	)
	r := newBlockReconciler(c)
	_, err := reconcileBlock(t, r, "fast")
	require.NoError(t, err)
	before := getBlock(t, c, "fast").ResourceVersion

	_, err = reconcileBlock(t, r, "fast")
	require.NoError(t, err)
	assert.Equal(t, before, getBlock(t, c, "fast").ResourceVersion)
}

// A pure unit test of the immutable-field diff storageclass_apply.go uses to
// decide whether an existing StorageClass can be reconciled in place or is a
// terminal conflict.
func TestImmutableStorageClassDrift(t *testing.T) {
	t.Parallel()
	desired := RenderStorageClass("ceph-pool", testNamespace, "ceph-pool", testNamespace)

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
