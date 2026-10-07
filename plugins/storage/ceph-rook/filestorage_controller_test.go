package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// Every test in this file reconciles one FileStorage named "shared".
const fileName = "shared"

func testFileStorage() *v1alpha1.FileStorage {
	return &v1alpha1.FileStorage{
		ObjectMeta: metav1.ObjectMeta{Name: fileName, UID: types.UID("uid-" + fileName)},
		Spec:       v1alpha1.FileStorageSpec{MetadataServers: 1},
	}
}

func newFileReconciler(c client.Client) *ConsumerReconciler[*v1alpha1.FileStorage] {
	return NewFileStorageReconciler(c, testNamespace, testNamespace, "")
}

// The generic ConsumerReconciler contract is asserted by the shared suite;
// this file keeps only FileStorage-specific behavior.
func TestFileStorageSuite(t *testing.T) {
	t.Parallel()
	runConsumerSuite(t, consumerSuite[*v1alpha1.FileStorage]{
		newObject: testFileStorage,
		withReplication3: func() *v1alpha1.FileStorage {
			fs := testFileStorage()
			fs.Spec.Replicas = ptr.To[int32](3)
			return fs
		},
		newReconciler: newFileReconciler,
		sizePath:      []string{"spec", "metadataPool", "replicated", "size"},
	})
}

// FileStorage-specific derived shape: spec.metadataServers reaches the
// CephFilesystem, and the CephFS StorageClass names Rook's derived data pool.
func TestFileStorageDerivesCephFSPair(t *testing.T) {
	t.Parallel()
	fsObj := testFileStorage()
	fsObj.Spec.MetadataServers = 2
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		fsObj,
	)
	r := newFileReconciler(c)

	_, err := r.Reconcile(context.Background(), reconcileRequest(fileName))
	require.NoError(t, err)

	cfs := rookStub("CephFilesystem")
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: testNamespace, Name: "cephfs-shared"}, cfs))
	active, _, _ := unstructured.NestedInt64(cfs.Object, "spec", "metadataServer", "activeCount")
	assert.Equal(t, int64(2), active)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "cephfs-shared"}, &sc))
	assert.Equal(t, "rook-ceph.cephfs.csi.ceph.com", sc.Provisioner)
	assert.Equal(t, "cephfs-shared", sc.Parameters["fsName"])
	assert.Equal(t, "cephfs-shared-data0", sc.Parameters["pool"])
}

// The CephFSMounter config must reach the derived StorageClass, so a cluster
// whose nodes lack the ceph kernel module can serve CephFS via ceph-fuse.
func TestFileStorageAppliesConfiguredMounter(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testFileStorage(),
	)
	r := NewFileStorageReconciler(c, testNamespace, testNamespace, "fuse")

	_, err := r.Reconcile(context.Background(), reconcileRequest(fileName))
	require.NoError(t, err)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "cephfs-shared"}, &sc))
	assert.Equal(t, "fuse", sc.Parameters["mounter"])
}
