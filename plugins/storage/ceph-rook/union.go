package main

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// resolvePoolDisks resolves one pool's spec.disks into their DiskStatus, with
// notes for anything skipped. Every skip rule lives here, shared between pool
// status (reconcilePool) and the CephCluster union (diskUnion), so the two can
// never disagree on what a pool contributes.
//
// Skipped: duplicate names (spec.disks is a set, but an object written before
// that CRD marker existed could repeat one), disks another pool has a stronger
// claim to, disks that do not exist, and disks whose status has not been
// written yet (upsertDisk's Create and Status().Update are two API calls; the
// Disk watch re-enqueues once the status lands). Any other error (e.g. a
// transient API failure) is returned so the caller can requeue, rather than
// silently lowering the disk count.
//
// A disk reporting available=false is NOT skipped: once Ceph consumes a device
// it stops looking empty, so dropping unavailable disks would pull live OSDs
// out of the CephCluster on the next reconcile.
func resolvePoolDisks(ctx context.Context, c client.Client, pool *v1alpha1.DiskPool, allPools []v1alpha1.DiskPool) ([]v1alpha1.DiskStatus, []string, error) {
	var (
		selected     []v1alpha1.DiskStatus
		missing      []string
		conflicts    []string
		undiscovered []string
	)
	seen := make(map[string]struct{}, len(pool.Spec.Disks))
	for _, name := range pool.Spec.Disks {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		if owner := ClaimOwner(allPools, name); owner != "" && owner != pool.Name {
			conflicts = append(conflicts, fmt.Sprintf("%s (claimed by %s)", name, owner))
			continue
		}
		var disk v1alpha1.Disk
		err := c.Get(ctx, types.NamespacedName{Name: name}, &disk)
		if apierrors.IsNotFound(err) {
			missing = append(missing, name)
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get Disk %q: %w", name, err)
		}
		if disk.Status.Node == "" {
			undiscovered = append(undiscovered, name)
			continue
		}
		selected = append(selected, disk.Status)
	}

	var notes []string
	if len(missing) > 0 {
		notes = append(notes, "skipped missing disks: "+strings.Join(missing, ", "))
	}
	if len(conflicts) > 0 {
		notes = append(notes, "skipped disks claimed by another pool: "+strings.Join(conflicts, ", "))
	}
	if len(undiscovered) > 0 {
		notes = append(notes, "awaiting discovery: "+strings.Join(undiscovered, ", "))
	}
	return selected, notes, nil
}

// diskUnion resolves every live DiskPool's disks into one deduplicated
// list: the disks behind every OSD in the cluster. reconcileCephClusterNodes
// writes it into the CephCluster; consumer reconcilers size replication on it.
func diskUnion(ctx context.Context, c client.Client) ([]v1alpha1.DiskStatus, error) {
	var pools v1alpha1.DiskPoolList
	if err := c.List(ctx, &pools); err != nil {
		return nil, fmt.Errorf("list DiskPools: %w", err)
	}
	return diskUnionOver(ctx, c, pools.Items)
}

// diskUnionOver is diskUnion over an already-listed pool set, so a caller that
// holds the list (reconcilePool) does not fetch it twice.
func diskUnionOver(ctx context.Context, c client.Client, pools []v1alpha1.DiskPool) ([]v1alpha1.DiskStatus, error) {
	// Deduplicate by (node, device ref) so a disk listed in multiple pools --
	// or two Disk CRs resolving to one device -- isn't doubled.
	seen := make(map[string]struct{})
	var union []v1alpha1.DiskStatus
	for i := range pools {
		pool := &pools[i]
		// A pool being deleted has released its disks.
		if !pool.DeletionTimestamp.IsZero() {
			continue
		}
		selected, _, err := resolvePoolDisks(ctx, c, pool, pools)
		if err != nil {
			return nil, err
		}
		for j := range selected {
			st := &selected[j]
			key := st.Node + "\x00" + DeviceRef(st)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				union = append(union, *st)
			}
		}
	}
	return union, nil
}
