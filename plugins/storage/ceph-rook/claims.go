package main

import (
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// Prefix for the Kubernetes objects a StoragePool derives. A StoragePool's name
// is operator-chosen and the derived StorageClass is cluster-scoped, so an
// unprefixed name would collide with — and, through CreateOrUpdate, adopt and
// later garbage-collect — a pre-existing StorageClass such as k3d's
// "local-path". The prefix keeps the derived names in this plugin's own space;
// ownership is still verified before any write (see ownedByPool).
const derivedNamePrefix = "ceph-"

// DerivedName is the name of the CephBlockPool and StorageClass a StoragePool
// owns. Both are derived from the same string so the pool's status can report a
// single StorageClass name.
func DerivedName(poolName string) string {
	return derivedNamePrefix + poolName
}

// ClaimOwner returns the name of the StoragePool entitled to a disk when more
// than one lists it, or "" when no live pool claims it.
//
// Disk.status.claimedBy is written by the DiskInventory reconciler and can lag,
// so pool-vs-pool conflicts are decided here from the StoragePool list itself,
// which is authoritative. The oldest pool wins; equal timestamps fall back to
// the lexicographically first name so two controllers reach the same answer.
// Pools being deleted release their claims immediately.
func ClaimOwner(pools []v1alpha1.StoragePool, diskName string) string {
	var claimants []v1alpha1.StoragePool
	for _, pool := range pools {
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

// BuildClaimIndex maps every claimed disk name to its owning StoragePool,
// applying the same precedence as ClaimOwner. It is what populates
// Disk.status.claimedBy.
func BuildClaimIndex(pools []v1alpha1.StoragePool) map[string]string {
	index := make(map[string]string)
	for _, pool := range pools {
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

// ownedByPool reports whether refs already contain a controller reference to
// pool. Every write to a derived object is gated on this: an object that exists
// but is not ours must not be adopted, because adopting it would also mean
// deleting it when the pool goes away.
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
