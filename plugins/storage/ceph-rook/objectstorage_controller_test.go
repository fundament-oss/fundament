package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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
	return NewObjectStorageReconciler(c, testNamespace, testNamespace, "nl-1")
}

// The generic ConsumerReconciler contract is asserted by the shared suite;
// this file keeps only ObjectStorage-specific behavior.
func TestObjectStorageSuite(t *testing.T) {
	t.Parallel()
	runConsumerSuite(t, consumerSuite[*v1alpha1.ObjectStorage]{
		newObject: testObjectStorage,
		withReplication3: func() *v1alpha1.ObjectStorage {
			os := testObjectStorage()
			os.Spec.Replicas = ptr.To[int32](3)
			return os
		},
		newReconciler: newObjectReconciler,
		sizePath:      []string{"spec", "dataPool", "replicated", "size"},
	})
}

// A zero gatewayInstances (reachable for Go-constructed objects or during
// version skew, like the replicas guard) must not render a storeless RGW.
func TestObjectStorageFloorsZeroGatewayInstances(t *testing.T) {
	t.Parallel()
	osObj := testObjectStorage()
	osObj.Spec.GatewayInstances = 0
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		osObj,
	)
	r := newObjectReconciler(c)

	_, err := r.Reconcile(context.Background(), reconcileRequest(objectName))
	require.NoError(t, err)

	cos := rookStub("CephObjectStore")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "cephobj-main"}, cos))
	instances, _, _ := unstructured.NestedInt64(cos.Object, "spec", "gateway", "instances")
	assert.Equal(t, int64(1), instances, "0 RGW pods would never serve; floor to 1")
}

// ObjectStorage-specific derived shape: spec.gatewayInstances reaches the
// CephObjectStore, and the StorageClass drives Rook's OBC provisioner with
// the store's name and namespace.
func TestObjectStorageDerivesBucketPair(t *testing.T) {
	t.Parallel()
	osObj := testObjectStorage()
	osObj.Spec.GatewayInstances = 2
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		osObj,
	)
	r := newObjectReconciler(c)

	_, err := r.Reconcile(context.Background(), reconcileRequest(objectName))
	require.NoError(t, err)

	cos := rookStub("CephObjectStore")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "cephobj-main"}, cos))
	instances, _, _ := unstructured.NestedInt64(cos.Object, "spec", "gateway", "instances")
	assert.Equal(t, int64(2), instances)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "cephobj-main"}, &sc))
	assert.Equal(t, "rook-ceph.ceph.rook.io/bucket", sc.Provisioner)
	assert.Equal(t, "cephobj-main", sc.Parameters["objectStoreName"])
	assert.Equal(t, testNamespace, sc.Parameters["objectStoreNamespace"])
	assert.Equal(t, "nl-1", sc.Parameters["region"], "the configured signing region reaches the class")
}

func testObjectBucket(name, storageClass string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("objectbucket.io/v1alpha1")
	u.SetKind("ObjectBucket")
	u.SetName(name)
	require := map[string]any{"storageClassName": storageClass}
	u.Object["spec"] = require
	return u
}

// The finalizer is added on reconcile, so deletion ordering can be enforced.
func TestObjectStorageAddsFinalizer(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t, cephCluster(), testObjectStorage())
	r := newObjectReconciler(c)

	_, err := r.Reconcile(context.Background(), reconcileRequest(objectName))
	require.NoError(t, err)

	got := &v1alpha1.ObjectStorage{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: objectName}, got))
	assert.Contains(t, got.Finalizers, objectStorageFinalizer)
}

// Deleting while buckets still reference the derived StorageClass must not
// cascade the CephObjectStore and class away: the claims' finalizers need
// both to deprovision, and losing them first wedges every claim forever.
func TestObjectStorageDeleteBlockedByBuckets(t *testing.T) {
	t.Parallel()
	osObj := testObjectStorage()
	osObj.Finalizers = []string{objectStorageFinalizer}
	now := metav1.Now()
	osObj.DeletionTimestamp = &now
	c := newFakeClient(t,
		cephCluster(),
		osObj,
		testObjectBucket("obc-ns-bucket-1", "cephobj-main"),
	)
	r := newObjectReconciler(c)

	res, err := r.Reconcile(context.Background(), reconcileRequest(objectName))
	require.NoError(t, err)
	assert.Equal(t, provisioningRequeue, res.RequeueAfter, "poll until the buckets are gone")

	got := &v1alpha1.ObjectStorage{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: objectName}, got))
	assert.Contains(t, got.Finalizers, objectStorageFinalizer, "still blocked")
	assert.Equal(t, v1alpha1.PhaseDegraded, got.Status.Phase)
	assert.Contains(t, got.Status.Message, "bucket")

	// A bucket on another store does not block; with the referencing one
	// gone the finalizer is released and the object deletes.
	require.NoError(t, c.Delete(context.Background(), testObjectBucket("obc-ns-bucket-1", "cephobj-main")))
	_, err = r.Reconcile(context.Background(), reconcileRequest(objectName))
	require.NoError(t, err)
	getErr := c.Get(context.Background(), types.NamespacedName{Name: objectName}, got)
	assert.True(t, apierrors.IsNotFound(getErr), "finalizer released, object gone")
}
