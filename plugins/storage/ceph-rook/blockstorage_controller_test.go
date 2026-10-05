package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

func testBlockStorage(name string) *v1alpha1.BlockStorage {
	return &v1alpha1.BlockStorage{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("uid-" + name)},
		Spec:       v1alpha1.BlockStorageSpec{},
	}
}

func newBlockReconciler(c client.Client) *ConsumerReconciler[*v1alpha1.BlockStorage] {
	return NewBlockStorageReconciler(c, testNamespace, testNamespace)
}

// Every test in this file reconciles one BlockStorage named "fast".
const blockName = "fast"

func reconcileBlock(t *testing.T, r *ConsumerReconciler[*v1alpha1.BlockStorage]) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), reconcileRequest(blockName))
}

func getBlock(t *testing.T, c client.Client) *v1alpha1.BlockStorage {
	t.Helper()
	var bs v1alpha1.BlockStorage
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: blockName}, &bs))
	return &bs
}

// The generic ConsumerReconciler contract — derived-object ownership,
// degradation, adoption refusal, drift, status discipline — is asserted by
// the shared suite; this file keeps only BlockStorage-specific behavior.
func TestBlockStorageSuite(t *testing.T) {
	t.Parallel()
	runConsumerSuite(t, consumerSuite[*v1alpha1.BlockStorage]{
		newObject: func() *v1alpha1.BlockStorage { return testBlockStorage(blockName) },
		withReplication3: func() *v1alpha1.BlockStorage {
			bs := testBlockStorage(blockName)
			bs.Spec.Replicas = ptr.To[int32](3)
			return bs
		},
		newReconciler: newBlockReconciler,
		sizePath:      []string{"spec", "replicated", "size"},
	})
}

// BlockStorage-specific derived shape: the RBD CSI StorageClass and its pool
// parameter.
func TestBlockStorageDerivesRBDStorageClass(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		testBlockStorage(blockName),
	)
	r := newBlockReconciler(c)

	_, err := reconcileBlock(t, r)
	require.NoError(t, err)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "ceph-fast"}, &sc))
	assert.Equal(t, "rook-ceph.rbd.csi.ceph.com", sc.Provisioner)
	assert.Equal(t, "ceph-fast", sc.Parameters["pool"])
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
	expandable.AllowVolumeExpansion = new(false)
	assert.Empty(t, immutableStorageClassDrift(expandable, desired))
}

// spec.default marks the derived StorageClass as the cluster default, so PVCs
// without an explicit storageClassName land on Ceph instead of the
// distribution's own default class.
func TestBlockStorageDefaultAnnotatesStorageClass(t *testing.T) {
	t.Parallel()
	bs := testBlockStorage(blockName)
	bs.Spec.Default = true
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		bs,
	)
	r := newBlockReconciler(c)

	_, err := reconcileBlock(t, r)
	require.NoError(t, err)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "ceph-fast"}, &sc))
	assert.Equal(t, "true", sc.Annotations["storageclass.kubernetes.io/is-default-class"])
	assert.Equal(t, v1alpha1.PhaseProvisioning, getBlock(t, c).Status.Phase)
}

// Two BlockStorages claiming default is a configuration error: neither
// StorageClass gets the annotation and both report Degraded naming the other,
// so the conflict cannot silently pick a winner. Provisioning itself keeps
// working: the derived pool and StorageClass are still applied.
func TestBlockStorageDefaultConflictDegrades(t *testing.T) {
	t.Parallel()
	fast := testBlockStorage(blockName)
	fast.Spec.Default = true
	other := testBlockStorage("other")
	other.Spec.Default = true
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		fast,
		other,
	)
	r := newBlockReconciler(c)

	_, err := reconcileBlock(t, r)
	require.NoError(t, err)

	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "ceph-fast"}, &sc),
		"the StorageClass is still applied; only the default annotation is withheld")
	assert.NotContains(t, sc.Annotations, "storageclass.kubernetes.io/is-default-class")

	bs := getBlock(t, c)
	assert.Equal(t, v1alpha1.PhaseDegraded, bs.Status.Phase)
	assert.Contains(t, bs.Status.Message, "other")
	cond := meta.FindStatusCondition(bs.Status.Conditions, v1alpha1.ConditionReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha1.ReasonDefaultConflict, cond.Reason)
}

// Unsetting spec.default must take the annotation back off the StorageClass,
// and must not disturb annotations someone else put there.
func TestBlockStorageRemovesDefaultAnnotationWhenUnset(t *testing.T) {
	t.Parallel()
	bs := testBlockStorage(blockName)
	bs.Spec.Default = true
	c := newFakeClient(t,
		cephCluster(),
		testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
		testPool("pool", time.Now(), "node-a-1"),
		bs,
	)
	r := newBlockReconciler(c)
	_, err := reconcileBlock(t, r)
	require.NoError(t, err)

	// A foreign annotation lands on the live object between reconciles.
	var sc storagev1.StorageClass
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "ceph-fast"}, &sc))
	require.Equal(t, "true", sc.Annotations["storageclass.kubernetes.io/is-default-class"])
	sc.Annotations["example.com/keep"] = "yes"
	require.NoError(t, c.Update(context.Background(), &sc))

	live := getBlock(t, c)
	live.Spec.Default = false
	require.NoError(t, c.Update(context.Background(), live))

	_, err = reconcileBlock(t, r)
	require.NoError(t, err)

	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "ceph-fast"}, &sc))
	assert.NotContains(t, sc.Annotations, "storageclass.kubernetes.io/is-default-class")
	assert.Equal(t, "yes", sc.Annotations["example.com/keep"], "foreign annotations survive")
}
