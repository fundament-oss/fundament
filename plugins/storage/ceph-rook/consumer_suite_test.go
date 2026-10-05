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

// consumerSuite parameterizes the shared behavior of every consumer kind, so
// the generic ConsumerReconciler's contract is asserted once for all of them;
// per-kind test files keep only kind-specific assertions.
type consumerSuite[T consumer] struct {
	// newObject returns the consumer with default spec and a stable UID. The
	// kind, CR name and derived name are read off the reconciler and this
	// object, so the suite asserts against exactly the names production uses.
	newObject func() T
	// withReplication3 returns the consumer explicitly requesting 3 replicas.
	withReplication3 func() T
	newReconciler    func(c client.Client) *ConsumerReconciler[T]
	// sizePath locates the replicated size field inside the derived Rook
	// object's spec, for the foreign-field preservation scenario.
	sizePath []string
}

// reconcileRequest builds a cluster-scoped reconcile request by name.
func reconcileRequest(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name}}
}

func runConsumerSuite[T consumer](t *testing.T, s consumerSuite[T]) {
	// A reconciler without a client, for its renderers and naming config.
	template := s.newReconciler(nil)
	rookKind := template.rookKind
	kind := template.kind
	crName := s.newObject().GetName()
	derived := template.derivedName(crName)

	reconcileIt := func(t *testing.T, r *ConsumerReconciler[T]) (ctrl.Result, error) {
		t.Helper()
		return r.Reconcile(context.Background(), reconcileRequest(crName))
	}
	get := func(t *testing.T, c client.Client) T {
		t.Helper()
		obj := s.newObject()
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: crName}, obj))
		return obj
	}
	getRook := func(t *testing.T, c client.Client) *unstructured.Unstructured {
		t.Helper()
		u := rookStub(rookKind)
		require.NoError(t, c.Get(context.Background(),
			types.NamespacedName{Namespace: testNamespace, Name: derived}, u))
		return u
	}
	getSC := func(t *testing.T, c client.Client) *storagev1.StorageClass {
		t.Helper()
		var sc storagev1.StorageClass
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: derived}, &sc))
		return &sc
	}
	renderOwnSC := func() *storagev1.StorageClass {
		return template.renderStorageClass(derived, testNamespace, derived, testNamespace)
	}

	// Happy path: derived Rook object + StorageClass, both controller-owned,
	// replication sized on the union, status populated.
	t.Run("creates owned derived objects", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testDisk("node-b-1", "node-b", "/dev/sdb", 200, true),
			testPool("pool", time.Now(), "node-a-1", "node-b-1"),
			s.newObject(),
		)
		r := s.newReconciler(c)

		res, err := reconcileIt(t, r)
		require.NoError(t, err)
		assert.Equal(t, provisioningRequeue, res.RequeueAfter)

		rook := getRook(t, c)
		require.Len(t, rook.GetOwnerReferences(), 1)
		assert.Equal(t, kind, rook.GetOwnerReferences()[0].Kind)

		sc := getSC(t, c)
		require.Len(t, sc.OwnerReferences, 1)
		assert.Equal(t, kind, sc.OwnerReferences[0].Kind)
		assert.Equal(t, crName, sc.OwnerReferences[0].Name)

		status := get(t, c).ConsumerStatus()
		assert.Equal(t, v1alpha1.PhaseProvisioning, status.Phase)
		assert.Equal(t, derived, status.StorageClassName)
		assert.Equal(t, 2, status.Replicas)
		assert.Equal(t, "host", status.FailureDomain)
	})

	// A nil reclaimPolicy or volumeBindingMode reads back as the server
	// default, which the drift check treats as immutable drift forever; the
	// fake client applies no defaulting, so only this invariant catches it.
	t.Run("renders defaulted immutable StorageClass fields explicitly", func(t *testing.T) {
		t.Parallel()
		sc := renderOwnSC()
		assert.NotNil(t, sc.ReclaimPolicy)
		assert.NotNil(t, sc.VolumeBindingMode)
	})

	// No DiskPool contributes disks: Degraded, nothing created.
	t.Run("degraded without OSDs", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t, cephCluster(), s.newObject())
		r := s.newReconciler(c)

		_, err := reconcileIt(t, r)
		require.NoError(t, err)

		var sc storagev1.StorageClass
		getErr := c.Get(context.Background(), types.NamespacedName{Name: derived}, &sc)
		assert.True(t, apierrors.IsNotFound(getErr), "no StorageClass without OSDs")

		status := get(t, c).ConsumerStatus()
		assert.Equal(t, v1alpha1.PhaseDegraded, status.Phase)
		assert.Contains(t, status.Message, "no DiskPool contributes disks")
		cond := meta.FindStatusCondition(status.Conditions, v1alpha1.ConditionReady)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, v1alpha1.ReasonNoOSDs, cond.Reason)
	})

	// Without the CephCluster nothing can provision; the real cause is named
	// and polled for — no watch covers the CephCluster here.
	t.Run("degraded without CephCluster", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t,
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.newObject(),
		)
		r := s.newReconciler(c)

		res, err := reconcileIt(t, r)
		require.NoError(t, err)
		assert.Equal(t, provisioningRequeue, res.RequeueAfter, "poll until the CephCluster exists")

		var sc storagev1.StorageClass
		getErr := c.Get(context.Background(), types.NamespacedName{Name: derived}, &sc)
		assert.True(t, apierrors.IsNotFound(getErr))

		status := get(t, c).ConsumerStatus()
		assert.Equal(t, v1alpha1.PhaseDegraded, status.Phase)
		assert.Contains(t, status.Message, "CephCluster")
		cond := meta.FindStatusCondition(status.Conditions, v1alpha1.ConditionReady)
		require.NotNil(t, cond)
		assert.Equal(t, v1alpha1.ReasonCephClusterMissing, cond.Reason)
	})

	// Adopting a StorageClass we do not own would delete it with the owner.
	t.Run("refuses foreign StorageClass", func(t *testing.T) {
		t.Parallel()
		foreign := &storagev1.StorageClass{
			ObjectMeta:  metav1.ObjectMeta{Name: derived},
			Provisioner: "rancher.io/local-path",
		}
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.newObject(),
			foreign,
		)
		r := s.newReconciler(c)

		_, err := reconcileIt(t, r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not owned by this "+kind)
		assert.True(t, errors.Is(err, reconcile.TerminalError(nil)),
			"refusing to adopt a foreign object is not retryable")
		assert.Equal(t, v1alpha1.PhaseDegraded, get(t, c).ConsumerStatus().Phase)
	})

	// Same rule for the derived Rook kind in the cluster namespace.
	t.Run("refuses foreign Rook object", func(t *testing.T) {
		t.Parallel()
		foreign := rookStub(rookKind)
		foreign.SetName(derived)
		foreign.SetNamespace(testNamespace)
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.newObject(),
			foreign,
		)
		r := s.newReconciler(c)

		_, err := reconcileIt(t, r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), rookKind)
		assert.Contains(t, err.Error(), "not owned by this "+kind)
		assert.True(t, errors.Is(err, reconcile.TerminalError(nil)))
		assert.Equal(t, v1alpha1.PhaseDegraded, get(t, c).ConsumerStatus().Phase)
	})

	// Reconciling an owned StorageClass must upsert our controller ref, not
	// replace the list: a foreign non-controller ownerRef survives.
	t.Run("keeps foreign ownerRef on StorageClass", func(t *testing.T) {
		t.Parallel()
		obj := s.newObject()
		sc := renderOwnSC()
		sc.OwnerReferences = []metav1.OwnerReference{
			controllerRef(obj, kind),
			{APIVersion: "v1", Kind: "ConfigMap", Name: "tracker", UID: types.UID("uid-tracker")},
		}
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			obj,
			sc,
		)
		r := s.newReconciler(c)

		_, err := reconcileIt(t, r)
		require.NoError(t, err)

		got := getSC(t, c)
		require.Len(t, got.OwnerReferences, 2, "the foreign non-controller ownerRef must survive")
		assert.True(t, ownedBy(got.OwnerReferences, kind, obj))
	})

	// Same for the derived Rook object: a stale spec forces the update path,
	// which must keep the existing ownerReferences.
	t.Run("keeps foreign ownerRef on Rook object", func(t *testing.T) {
		t.Parallel()
		obj := s.newObject()
		stale := template.renderRook(obj, testNamespace, derived, 3, "host")
		stale.SetOwnerReferences([]metav1.OwnerReference{
			controllerRef(obj, kind),
			{APIVersion: "v1", Kind: "ConfigMap", Name: "tracker", UID: types.UID("uid-tracker")},
		})
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			obj,
			stale,
		)
		r := s.newReconciler(c)

		_, err := reconcileIt(t, r)
		require.NoError(t, err)

		got := getRook(t, c)
		require.Len(t, got.GetOwnerReferences(), 2, "the foreign non-controller ownerRef must survive")
		assert.True(t, ownedBy(got.GetOwnerReferences(), kind, obj))
	})

	// The derived Rook object reporting Ready flips phase and condition.
	t.Run("ready when Rook object reports Ready", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.newObject(),
		)
		r := s.newReconciler(c)
		_, err := reconcileIt(t, r)
		require.NoError(t, err)

		rook := getRook(t, c)
		require.NoError(t, unstructured.SetNestedField(rook.Object, "Ready", "status", "phase"))
		require.NoError(t, c.Update(context.Background(), rook))

		res, err := reconcileIt(t, r)
		require.NoError(t, err)
		assert.Zero(t, res.RequeueAfter, "Ready needs no requeue")

		status := get(t, c).ConsumerStatus()
		assert.Equal(t, v1alpha1.PhaseReady, status.Phase)
		cond := meta.FindStatusCondition(status.Conditions, v1alpha1.ConditionReady)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
	})

	// Rook reporting Failure is a dead end, not progress: Degraded, with a
	// requeue in case Rook recovers.
	t.Run("degraded when Rook object fails", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.newObject(),
		)
		r := s.newReconciler(c)
		_, err := reconcileIt(t, r)
		require.NoError(t, err)

		rook := getRook(t, c)
		require.NoError(t, unstructured.SetNestedField(rook.Object, "Failure", "status", "phase"))
		require.NoError(t, c.Update(context.Background(), rook))

		res, err := reconcileIt(t, r)
		require.NoError(t, err)
		assert.Equal(t, provisioningRequeue, res.RequeueAfter, "recheck in case Rook recovers")

		status := get(t, c).ConsumerStatus()
		assert.Equal(t, v1alpha1.PhaseDegraded, status.Phase)
		assert.Contains(t, status.Message, "Failure")
		cond := meta.FindStatusCondition(status.Conditions, v1alpha1.ConditionReady)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, v1alpha1.ReasonRookFailure, cond.Reason)
	})

	// Explicit replication clamps to the contributing node count.
	t.Run("clamps explicit replication to nodes", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.withReplication3(),
		)
		r := s.newReconciler(c)
		_, err := reconcileIt(t, r)
		require.NoError(t, err)

		status := get(t, c).ConsumerStatus()
		assert.Equal(t, 1, status.Replicas)
		assert.Contains(t, status.Message, "clamped")
	})

	// Identical status must not be rewritten (Disk events fan out to every
	// consumer).
	t.Run("does not rewrite unchanged status", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.newObject(),
		)
		r := s.newReconciler(c)
		_, err := reconcileIt(t, r)
		require.NoError(t, err)
		before := get(t, c).GetResourceVersion()

		_, err = reconcileIt(t, r)
		require.NoError(t, err)
		assert.Equal(t, before, get(t, c).GetResourceVersion())
	})

	// Identical derived objects must not be rewritten either.
	t.Run("does not rewrite unchanged derived objects", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.newObject(),
		)
		r := s.newReconciler(c)
		_, err := reconcileIt(t, r)
		require.NoError(t, err)
		sc := getSC(t, c)
		rook := getRook(t, c)

		_, err = reconcileIt(t, r)
		require.NoError(t, err)
		assert.Equal(t, sc.ResourceVersion, getSC(t, c).ResourceVersion, "unchanged StorageClass rewritten")
		assert.Equal(t, rook.GetResourceVersion(), getRook(t, c).GetResourceVersion(), "unchanged %s rewritten", rookKind)
	})

	// An owned StorageClass whose immutable fields differ is a terminal error
	// naming the one recovery, and must not be updated in place.
	t.Run("immutable StorageClass drift is terminal", func(t *testing.T) {
		t.Parallel()
		obj := s.newObject()
		drifted := renderOwnSC()
		drifted.OwnerReferences = []metav1.OwnerReference{controllerRef(obj, kind)}
		drifted.Parameters = map[string]string{"drifted": "elsewhere"}
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			obj,
			drifted,
		)
		r := s.newReconciler(c)

		_, err := reconcileIt(t, r)
		require.Error(t, err)
		assert.True(t, errors.Is(err, reconcile.TerminalError(nil)),
			"retrying cannot change an immutable field")
		assert.Contains(t, err.Error(), "Delete the StorageClass")

		got := getSC(t, c)
		assert.Equal(t, map[string]string{"drifted": "elsewhere"}, got.Parameters,
			"no update may be issued against a drifted StorageClass")
		assert.Equal(t, v1alpha1.PhaseDegraded, get(t, c).ConsumerStatus().Phase)
	})

	// Fields the renderer does not emit — CRD defaults, Rook-written values —
	// survive updates, and their presence alone is not drift.
	t.Run("preserves foreign spec fields on Rook object", func(t *testing.T) {
		t.Parallel()
		c := newFakeClient(t,
			cephCluster(),
			testDisk("node-a-1", "node-a", "/dev/sdb", 100, true),
			testPool("pool", time.Now(), "node-a-1"),
			s.newObject(),
		)
		r := s.newReconciler(c)
		_, err := reconcileIt(t, r)
		require.NoError(t, err)

		// Simulate defaulting at levels the renderer omits: one top-level
		// field, one next to the replicated size.
		nestedForeign := append(append([]string{}, s.sizePath[:len(s.sizePath)-1]...), "targetSizeRatio")
		rook := getRook(t, c)
		require.NoError(t, unstructured.SetNestedField(rook.Object, false, "spec", "foreignTopLevel"))
		require.NoError(t, unstructured.SetNestedField(rook.Object, "0", nestedForeign...))
		require.NoError(t, c.Update(context.Background(), rook))
		defaulted := rook.GetResourceVersion()

		_, err = reconcileIt(t, r)
		require.NoError(t, err)

		after := getRook(t, c)
		assert.Equal(t, defaulted, after.GetResourceVersion(),
			"defaulted fields alone are not drift; no update may be issued")

		// A real change reconciles the rendered field back and keeps both
		// foreign ones.
		require.NoError(t, unstructured.SetNestedField(after.Object, int64(99), s.sizePath...))
		require.NoError(t, c.Update(context.Background(), after))

		_, err = reconcileIt(t, r)
		require.NoError(t, err)

		final := getRook(t, c)
		size, _, err := unstructured.NestedInt64(final.Object, s.sizePath...)
		require.NoError(t, err)
		assert.EqualValues(t, 1, size, "the rendered field is reconciled back")
		_, found, err := unstructured.NestedBool(final.Object, "spec", "foreignTopLevel")
		require.NoError(t, err)
		assert.True(t, found, "top-level foreign field survives the update")
		ratio, found, err := unstructured.NestedString(final.Object, nestedForeign...)
		require.NoError(t, err)
		assert.True(t, found, "nested foreign field survives the update")
		assert.Equal(t, "0", ratio)
	})

	// Removing a conflicting foreign object is notOursError's documented
	// recovery; its deletion must map back to the same-named consumer.
	t.Run("conflicting object event maps to same-named consumer", func(t *testing.T) {
		t.Parallel()
		// A second consumer proves the mapping enqueues only the same-named
		// one, not every consumer of the kind.
		other := s.newObject()
		other.SetName("other")
		other.SetUID(types.UID("uid-other"))
		c := newFakeClient(t, s.newObject(), other)
		r := s.newReconciler(c)

		foreign := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: derived}}
		reqs := r.toSameNamedConsumer(context.Background(), foreign)
		require.Len(t, reqs, 1)
		assert.Equal(t, crName, reqs[0].Name)

		unrelated := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}}
		assert.Empty(t, r.toSameNamedConsumer(context.Background(), unrelated))
	})
}
