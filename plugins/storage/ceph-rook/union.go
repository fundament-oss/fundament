package main

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/fundament-oss/fundament/plugins/storage/ceph-rook/api/v1alpha1"
)

// diskUnion resolves every live StoragePool's disks into one deduplicated
// list: the disks behind every OSD in the cluster. reconcileCephClusterNodes
// writes it into the CephCluster; consumer reconcilers size replication on it.
// Shared so the two can never disagree.
func diskUnion(ctx context.Context, c client.Client) ([]v1alpha1.DiskStatus, error) {
	var pools v1alpha1.StoragePoolList
	if err := c.List(ctx, &pools); err != nil {
		return nil, fmt.Errorf("list StoragePools: %w", err)
	}

	// Deduplicate by (node, device ref) so a disk listed in multiple pools --
	// or two Disk CRs resolving to one device -- isn't doubled.
	seen := make(map[string]struct{})
	var union []v1alpha1.DiskStatus
	for i := range pools.Items {
		pool := &pools.Items[i]
		// A pool being deleted has released its disks.
		if !pool.DeletionTimestamp.IsZero() {
			continue
		}
		for _, name := range pool.Spec.Disks {
			var disk v1alpha1.Disk
			err := c.Get(ctx, types.NamespacedName{Name: name}, &disk)
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("get Disk %q: %w", name, err)
			}
			key := disk.Status.Node + "\x00" + DeviceRef(&disk.Status)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				union = append(union, disk.Status)
			}
		}
	}
	return union, nil
}
