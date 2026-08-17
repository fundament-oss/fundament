package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

func poolAt(name string, created time.Time, disks ...string) v1alpha1.StoragePool {
	return v1alpha1.StoragePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               types.UID("uid-" + name),
			CreationTimestamp: metav1.NewTime(created),
		},
		Spec: v1alpha1.StoragePoolSpec{Disks: disks},
	}
}

func TestDerivedNameIsPrefixed(t *testing.T) {
	t.Parallel()
	// An unprefixed name could adopt something like k3d's "local-path".
	assert.Equal(t, "ceph-local-path", DerivedName("local-path"))
	assert.Equal(t, "ceph-default", DerivedName("default"))
}

func TestClaimOwner(t *testing.T) {
	t.Parallel()
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)

	tests := []struct {
		name  string
		pools []v1alpha1.StoragePool
		disk  string
		want  string
	}{
		{
			name:  "unclaimed disk has no owner",
			pools: []v1alpha1.StoragePool{poolAt("a", early, "disk-1")},
			disk:  "disk-2",
			want:  "",
		},
		{
			name:  "single claimant wins",
			pools: []v1alpha1.StoragePool{poolAt("a", early, "disk-1")},
			disk:  "disk-1",
			want:  "a",
		},
		{
			name:  "oldest pool wins regardless of list order",
			pools: []v1alpha1.StoragePool{poolAt("zeta", late, "disk-1"), poolAt("alpha", early, "disk-1")},
			disk:  "disk-1",
			want:  "alpha",
		},
		{
			name:  "equal timestamps fall back to lexicographic name",
			pools: []v1alpha1.StoragePool{poolAt("zeta", early, "disk-1"), poolAt("alpha", early, "disk-1")},
			disk:  "disk-1",
			want:  "alpha",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ClaimOwner(tc.pools, tc.disk))
		})
	}
}

// Release on delete, or the disk stays unusable until the object goes.
func TestClaimOwnerIgnoresDeletingPools(t *testing.T) {
	t.Parallel()
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	deleting := poolAt("old", early, "disk-1")
	deleting.DeletionTimestamp = ptr(metav1.NewTime(early.Add(time.Minute)))
	deleting.Finalizers = []string{"test/keep-visible"}

	live := poolAt("new", early.Add(time.Hour), "disk-1")

	assert.Equal(t, "new", ClaimOwner([]v1alpha1.StoragePool{deleting, live}, "disk-1"))
	assert.Empty(t, ClaimOwner([]v1alpha1.StoragePool{deleting}, "disk-1"))
}

func TestBuildClaimIndexAgreesWithClaimOwner(t *testing.T) {
	t.Parallel()
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pools := []v1alpha1.StoragePool{
		poolAt("zeta", early.Add(time.Hour), "disk-1", "disk-3"),
		poolAt("alpha", early, "disk-1", "disk-2"),
	}

	index := BuildClaimIndex(pools)

	assert.Equal(t, map[string]string{
		"disk-1": "alpha", // contested, oldest pool wins
		"disk-2": "alpha",
		"disk-3": "zeta",
	}, index)

	// The index must match ClaimOwner disk for disk, or the inventory and the
	// reconciler disagree on ownership.
	for disk, owner := range index {
		assert.Equal(t, ClaimOwner(pools, disk), owner, "disk %s", disk)
	}
}

func TestOwnedByPool(t *testing.T) {
	t.Parallel()
	pool := poolAt("mine", time.Now())
	yes, no := true, false

	assert.False(t, ownedByPool(nil, &pool), "no owner refs means not ours")
	assert.True(t, ownedByPool([]metav1.OwnerReference{{
		Kind: "StoragePool", Name: "mine", UID: pool.UID, Controller: &yes,
	}}, &pool))

	// A recreated pool has a new UID; the old refs are not ours.
	assert.False(t, ownedByPool([]metav1.OwnerReference{{
		Kind: "StoragePool", Name: "mine", UID: "stale-uid", Controller: &yes,
	}}, &pool))

	assert.False(t, ownedByPool([]metav1.OwnerReference{{
		Kind: "StoragePool", Name: "other", UID: "uid-other", Controller: &yes,
	}}, &pool))

	// A non-controller reference is not ownership.
	assert.False(t, ownedByPool([]metav1.OwnerReference{{
		Kind: "StoragePool", Name: "mine", UID: pool.UID, Controller: &no,
	}}, &pool))
}

func ptr[T any](v T) *T { return &v }
