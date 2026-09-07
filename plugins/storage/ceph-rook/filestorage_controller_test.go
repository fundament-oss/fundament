package main

import (
	"context"
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

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

func testFileStorage(name string) *v1alpha1.FileStorage {
	return &v1alpha1.FileStorage{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("uid-" + name)},
		Spec:       v1alpha1.FileStorageSpec{Replication: "auto", MetadataServers: 1},
	}
}

func newFileReconciler(c client.Client) *FileStorageReconciler {
	return &FileStorageReconciler{Client: c, ClusterNamespace: testNamespace, RookNamespace: testNamespace}
}

func reconcileFile(t *testing.T, r *FileStorageReconciler, name string) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: name}})
}

func getFile(t *testing.T, c client.Client, name string) *v1alpha1.FileStorage {
	t.Helper()
	var fs v1alpha1.FileStorage
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: name}, &fs))
	return &fs
}

func getCephFilesystem(t *testing.T, c client.Client, name string) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephFilesystem")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: name}, u))
	return u
}

// Happy path: cephfs-prefixed CephFilesystem + StorageClass, sized on the union.
func TestFileStorageCreatesDerivedObjects(t *testing.T) {
	t.Parallel()
	fsObj := testFileStorage("shared")
	fsObj.Spec.MetadataServers = 2
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testDisk("node-b-1", "node-b", "/dev/sdb", 200, true),
		testPool("pool", time.Now(), "node-a-1", "node-b-1"),
		fsObj,
	)
	r := newFileReconciler(c)

	res, err := reconcileFile(t, r, "shared")
	require.NoError(t, err)
	assert.Equal(t, provisioningRequeue, res.RequeueAfter)

	cfs := getCephFilesystem(t, c, "cephfs-shared")
	active, _, _ := unstructured.NestedInt64(cfs.Object, "spec", "metadataServer", "activeCount")
	assert.Equal(t, int64(2), active)
	require.Len(t, cfs.GetOwnerReferences(), 1)
	assert.Equal(t, "FileStorage", cfs.GetOwnerReferences()[0].Kind)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "cephfs-shared"}, &sc))
	assert.Equal(t, "rook-ceph.cephfs.csi.ceph.com", sc.Provisioner)
	assert.Equal(t, "cephfs-shared", sc.Parameters["fsName"])
	assert.Equal(t, "cephfs-shared-data0", sc.Parameters["pool"])
	require.Len(t, sc.OwnerReferences, 1)
	assert.Equal(t, "FileStorage", sc.OwnerReferences[0].Kind)

	got := getFile(t, c, "shared")
	assert.Equal(t, v1alpha1.PhaseProvisioning, got.Status.Phase)
	assert.Equal(t, "cephfs-shared", got.Status.StorageClassName)
	assert.Equal(t, 2, got.Status.Replicas)
	assert.Equal(t, "host", got.Status.FailureDomain)
}

// No StoragePool contributes disks: Degraded, nothing created.
func TestFileStorageDegradedWithoutOSDs(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t, cephCluster(), testFileStorage("shared"))
	r := newFileReconciler(c)

	_, err := reconcileFile(t, r, "shared")
	require.NoError(t, err)

	var sc storagev1.StorageClass
	getErr := c.Get(context.Background(), types.NamespacedName{Name: "cephfs-shared"}, &sc)
	assert.True(t, apierrors.IsNotFound(getErr))

	got := getFile(t, c, "shared")
	assert.Equal(t, v1alpha1.PhaseDegraded, got.Status.Phase)
	assert.Contains(t, got.Status.Message, "create a StoragePool")
	cond := meta.FindStatusCondition(got.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, v1alpha1.ReasonNoOSDs, cond.Reason)
}

// Adoption refusal, same contract as every derived object.
func TestFileStorageRefusesForeignCephFilesystem(t *testing.T) {
	t.Parallel()
	foreign := &unstructured.Unstructured{}
	foreign.SetAPIVersion(cephAPIVersion)
	foreign.SetKind("CephFilesystem")
	foreign.SetName("cephfs-shared")
	foreign.SetNamespace(testNamespace)
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testFileStorage("shared"),
		foreign,
	)
	r := newFileReconciler(c)

	_, err := reconcileFile(t, r, "shared")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by this FileStorage")
	assert.Equal(t, v1alpha1.PhaseDegraded, getFile(t, c, "shared").Status.Phase)
}

// CephFilesystem reporting Ready flips phase and condition.
func TestFileStorageReadyWhenFilesystemReady(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testFileStorage("shared"),
	)
	r := newFileReconciler(c)
	_, err := reconcileFile(t, r, "shared")
	require.NoError(t, err)

	cfs := getCephFilesystem(t, c, "cephfs-shared")
	require.NoError(t, unstructured.SetNestedField(cfs.Object, "Ready", "status", "phase"))
	require.NoError(t, c.Update(context.Background(), cfs))

	res, err := reconcileFile(t, r, "shared")
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)

	got := getFile(t, c, "shared")
	assert.Equal(t, v1alpha1.PhaseReady, got.Status.Phase)
	cond := meta.FindStatusCondition(got.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

// Explicit replication clamps to the union's node count, like BlockStorage.
func TestFileStorageClampsReplicationToNodes(t *testing.T) {
	t.Parallel()
	fsObj := testFileStorage("shared")
	fsObj.Spec.Replication = "3"
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		fsObj,
	)
	r := newFileReconciler(c)
	_, err := reconcileFile(t, r, "shared")
	require.NoError(t, err)

	got := getFile(t, c, "shared")
	assert.Equal(t, 1, got.Status.Replicas)
	assert.Contains(t, got.Status.Message, "clamped")
}

// Identical status must not be rewritten.
func TestFileStorageDoesNotRewriteUnchangedStatus(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testFileStorage("shared"),
	)
	r := newFileReconciler(c)
	_, err := reconcileFile(t, r, "shared")
	require.NoError(t, err)
	before := getFile(t, c, "shared").ResourceVersion

	_, err = reconcileFile(t, r, "shared")
	require.NoError(t, err)
	assert.Equal(t, before, getFile(t, c, "shared").ResourceVersion)
}
