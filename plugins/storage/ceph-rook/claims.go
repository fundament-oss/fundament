package main

import (
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// Prefix for the objects a StoragePool derives. Pool names are operator-chosen
// and StorageClasses are cluster-scoped, so an unprefixed name could collide with
// — and then garbage-collect — something like k3d's "local-path". The prefix
// reduces collisions; ownedByPool is what prevents damage.
const derivedNamePrefix = "ceph-"

// DerivedName names both the CephBlockPool and the StorageClass, so status can
// report a single name.
func DerivedName(poolName string) string {
	return derivedNamePrefix + poolName
}

// ClaimOwner returns the StoragePool entitled to a disk when more than one lists
// it, or "" when no live pool claims it.
//
// Decided from the StoragePool list, not Disk.status.claimedBy, which lags.
// Oldest pool wins; ties break on name so two controllers agree. Pools being
// deleted release their claims immediately.
func ClaimOwner(pools []v1alpha1.StoragePool, diskName string) string {
	var claimants []*v1alpha1.StoragePool
	for i := range pools {
		pool := &pools[i]
		if !pool.DeletionTimestamp.IsZero() {
			continue
		}
		for _, d := range pool.Spec.Disks {
			if d == diskName {
				claimants = append(claimants, pool)
				break
			}
		}
	}
	if len(claimants) == 0 {
		return ""
	}
	sort.Slice(claimants, func(i, j int) bool {
		ti, tj := claimants[i].CreationTimestamp, claimants[j].CreationTimestamp
		if !ti.Equal(&tj) {
			return ti.Before(&tj)
		}
		return claimants[i].Name < claimants[j].Name
	})
	return claimants[0].Name
}

// BuildClaimIndex maps each claimed disk to its owning StoragePool using
// ClaimOwner's precedence. It populates Disk.status.claimedBy.
func BuildClaimIndex(pools []v1alpha1.StoragePool) map[string]string {
	index := make(map[string]string)
	for i := range pools {
		pool := &pools[i]
		if !pool.DeletionTimestamp.IsZero() {
			continue
		}
		for _, diskName := range pool.Spec.Disks {
			if _, done := index[diskName]; done {
				continue
			}
			index[diskName] = ClaimOwner(pools, diskName)
		}
	}
	return index
}

// ownedByPool reports whether refs contain a controller reference to pool. Every
// write to a derived object is gated on this: adopting an object we do not own
// would also mean deleting it when the pool goes away.
func ownedByPool(refs []metav1.OwnerReference, pool *v1alpha1.StoragePool) bool {
	for _, ref := range refs {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		if ref.Kind == "StoragePool" && ref.Name == pool.Name && ref.UID == pool.UID {
			return true
		}
	}
	return false
}
