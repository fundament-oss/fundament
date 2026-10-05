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

// Every test in this file reconciles one ObjectStorage named "main".
const objectName = "main"

func testObjectStorage() *v1alpha1.ObjectStorage {
	return &v1alpha1.ObjectStorage{
		ObjectMeta: metav1.ObjectMeta{Name: objectName, UID: types.UID("uid-" + objectName)},
		Spec:       v1alpha1.ObjectStorageSpec{GatewayInstances: 1},
	}
}

func newObjectReconciler(c client.Client) *ConsumerReconciler[*v1alpha1.ObjectStorage] {
	return NewObjectStorageReconciler(c, testNamespace, testNamespace)
}

func reconcileObject(t *testing.T, r *ConsumerReconciler[*v1alpha1.ObjectStorage]) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: objectName}})
}

func getObject(t *testing.T, c client.Client) *v1alpha1.ObjectStorage {
	t.Helper()
	var os v1alpha1.ObjectStorage
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: objectName}, &os))
	return &os
}

func getCephObjectStore(t *testing.T, c client.Client, name string) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(cephAPIVersion)
	u.SetKind("CephObjectStore")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: name}, u))
	return u
}

// Happy path: cephobj-prefixed CephObjectStore + bucket StorageClass, sized on
// the union.
func TestObjectStorageCreatesDerivedObjects(t *testing.T) {
	t.Parallel()
	osObj := testObjectStorage()
	osObj.Spec.GatewayInstances = 2
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testDisk("node-b-1", "node-b", "/dev/sdb", 200, true),
		testPool("pool", time.Now(), "node-a-1", "node-b-1"),
		osObj,
	)
	r := newObjectReconciler(c)

	res, err := reconcileObject(t, r)
	require.NoError(t, err)
	assert.Equal(t, provisioningRequeue, res.RequeueAfter)

	cos := getCephObjectStore(t, c, "cephobj-main")
	instances, _, _ := unstructured.NestedInt64(cos.Object, "spec", "gateway", "instances")
	assert.Equal(t, int64(2), instances)
	require.Len(t, cos.GetOwnerReferences(), 1)
	assert.Equal(t, "ObjectStorage", cos.GetOwnerReferences()[0].Kind)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "cephobj-main"}, &sc))
	assert.Equal(t, "rook-ceph.ceph.rook.io/bucket", sc.Provisioner)
	assert.Equal(t, "cephobj-main", sc.Parameters["objectStoreName"])
	assert.Equal(t, testNamespace, sc.Parameters["objectStoreNamespace"])
	require.Len(t, sc.OwnerReferences, 1)
	assert.Equal(t, "ObjectStorage", sc.OwnerReferences[0].Kind)

	got := getObject(t, c)
	assert.Equal(t, v1alpha1.PhaseProvisioning, got.Status.Phase)
	assert.Equal(t, "cephobj-main", got.Status.StorageClassName)
	assert.Equal(t, 2, got.Status.Replicas)
	assert.Equal(t, "host", got.Status.FailureDomain)
}

// No DiskPool contributes disks: Degraded, nothing created.
func TestObjectStorageDegradedWithoutOSDs(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t, cephCluster(), testObjectStorage())
	r := newObjectReconciler(c)

	_, err := reconcileObject(t, r)
	require.NoError(t, err)

	var sc storagev1.StorageClass
	getErr := c.Get(context.Background(), types.NamespacedName{Name: "cephobj-main"}, &sc)
	assert.True(t, apierrors.IsNotFound(getErr))

	got := getObject(t, c)
	assert.Equal(t, v1alpha1.PhaseDegraded, got.Status.Phase)
	assert.Contains(t, got.Status.Message, "no DiskPool contributes disks")
	cond := meta.FindStatusCondition(got.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, v1alpha1.ReasonNoOSDs, cond.Reason)
}

// Adoption refusal, same contract as every derived object.
func TestObjectStorageRefusesForeignCephObjectStore(t *testing.T) {
	t.Parallel()
	foreign := &unstructured.Unstructured{}
	foreign.SetAPIVersion(cephAPIVersion)
	foreign.SetKind("CephObjectStore")
	foreign.SetName("cephobj-main")
	foreign.SetNamespace(testNamespace)
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testObjectStorage(),
		foreign,
	)
	r := newObjectReconciler(c)

	_, err := reconcileObject(t, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by this ObjectStorage")
	assert.Equal(t, v1alpha1.PhaseDegraded, getObject(t, c).Status.Phase)
}

// CephObjectStore reporting Ready flips phase and condition. Rook v1.16's
// object controller sets status.phase to Ready on a successful reconcile
// ("Connected" is external-mode only).
func TestObjectStorageReadyWhenStoreReady(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testObjectStorage(),
	)
	r := newObjectReconciler(c)
	_, err := reconcileObject(t, r)
	require.NoError(t, err)

	cos := getCephObjectStore(t, c, "cephobj-main")
	require.NoError(t, unstructured.SetNestedField(cos.Object, "Ready", "status", "phase"))
	require.NoError(t, c.Update(context.Background(), cos))

	res, err := reconcileObject(t, r)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)

	got := getObject(t, c)
	assert.Equal(t, v1alpha1.PhaseReady, got.Status.Phase)
	cond := meta.FindStatusCondition(got.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

// Rook reporting Failure (e.g. RGW pods unschedulable) is a dead end, not
// progress: surface it as Degraded instead of Provisioning forever.
func TestObjectStorageDegradedWhenStoreFails(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testObjectStorage(),
	)
	r := newObjectReconciler(c)
	_, err := reconcileObject(t, r)
	require.NoError(t, err)

	cos := getCephObjectStore(t, c, "cephobj-main")
	require.NoError(t, unstructured.SetNestedField(cos.Object, "Failure", "status", "phase"))
	require.NoError(t, c.Update(context.Background(), cos))

	res, err := reconcileObject(t, r)
	require.NoError(t, err)
	assert.Equal(t, provisioningRequeue, res.RequeueAfter, "recheck in case Rook recovers")

	got := getObject(t, c)
	assert.Equal(t, v1alpha1.PhaseDegraded, got.Status.Phase)
	assert.Contains(t, got.Status.Message, "Failure")
	cond := meta.FindStatusCondition(got.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha1.ReasonRookFailure, cond.Reason)
}

// Explicit spec.replicas clamps to the union's node count, like the other
// consumer kinds.
func TestObjectStorageClampsReplicasToNodes(t *testing.T) {
	t.Parallel()
	osObj := testObjectStorage()
	three := int32(3)
	osObj.Spec.Replicas = &three
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		osObj,
	)
	r := newObjectReconciler(c)
	_, err := reconcileObject(t, r)
	require.NoError(t, err)

	got := getObject(t, c)
	assert.Equal(t, 1, got.Status.Replicas)
	assert.Contains(t, got.Status.Message, "clamped")
}

// Identical status must not be rewritten.
func TestObjectStorageDoesNotRewriteUnchangedStatus(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testObjectStorage(),
	)
	r := newObjectReconciler(c)
	_, err := reconcileObject(t, r)
	require.NoError(t, err)
	before := getObject(t, c).ResourceVersion

	_, err = reconcileObject(t, r)
	require.NoError(t, err)
	assert.Equal(t, before, getObject(t, c).ResourceVersion)
}
