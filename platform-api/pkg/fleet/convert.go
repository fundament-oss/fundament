package fleet

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/clusterstatus"
	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

// clusterStatus derives the status of a ClusterList row the way the console
// does (common/clusterstatus).
func clusterStatus(row *db.ClusterListRow) clusterstatus.Status {
	return deriveStatus(row.ShootStatus, row.ShootStatusMessage, row.ShootHealth,
		row.ShootUpdating, row.OutboxStatus, row.ReachedGardener)
}

// clusterstatusOf derives the status of a ClusterStatusInputList row, the
// same way as clusterStatus.
func clusterstatusOf(row *db.ClusterStatusInputListRow) clusterstatus.Status {
	return deriveStatus(row.ShootStatus, row.ShootStatusMessage, row.ShootHealth,
		row.ShootUpdating, row.OutboxStatus, row.ReachedGardener)
}

// deriveStatus is the one mapping onto clusterstatus.Input for both row
// types, so the list's UNHEALTHY filter and the unhealthy counts cannot
// drift apart: a new input is a new parameter, which both callers must pass.
// Both queries read live clusters only, so Deleted is never set.
func deriveStatus(shootStatus, shootMessage, shootHealth pgtype.Text, updating bool, outboxStatus pgtype.Text, reachedGardener bool) clusterstatus.Status {
	return clusterstatus.FromDB(clusterstatus.Input{
		ShootStatus:     shootStatus,
		ShootMessage:    shootMessage,
		ShootHealth:     shootHealth,
		Updating:        updating,
		OutboxStatus:    outboxStatus,
		ReachedGardener: reachedGardener,
	})
}

// unhealthy is the rule behind clusters_unhealthy and the UNHEALTHY filter.
func unhealthy(row *db.ClusterListRow) bool {
	return clusterStatus(row).NeedsAttention()
}

func clusterStatusToProto(status clusterstatus.Status) fleetv1.ClusterStatus {
	switch status {
	case clusterstatus.Unspecified:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	case clusterstatus.Provisioning:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case clusterstatus.Starting:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_STARTING
	case clusterstatus.Running:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_RUNNING
	case clusterstatus.Upgrading:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_UPGRADING
	case clusterstatus.Unhealthy:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY
	case clusterstatus.Error:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_ERROR
	case clusterstatus.Deleting:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_DELETING
	case clusterstatus.Stopped:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	case clusterstatus.Lost:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_LOST
	default:
		panic(fmt.Sprintf("unhandled cluster status %d", status))
	}
}

func timestampOrNil(t pgtype.Timestamptz) *timestamppb.Timestamp {
	if !t.Valid {
		return nil
	}
	return timestamppb.New(t.Time)
}

func stringOrNil(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func clusterSummaryFrom(row *db.ClusterListRow) *fleetv1.ClusterSummary {
	b := fleetv1.ClusterSummary_builder{
		Id:                row.ID.String(),
		OrganizationId:    row.OrganizationID.String(),
		OrganizationName:  row.OrganizationName,
		Name:              row.Name,
		Region:            row.Region,
		KubernetesVersion: row.KubernetesVersion,
		Status:            clusterStatusToProto(clusterStatus(row)),
		StatusMessage:     row.ShootStatusMessage.String,
		Created:           timestampOrNil(row.Created),
		NodePools:         row.NodePools,
		NodesMin:          row.NodesMin,
		NodesMax:          row.NodesMax,
		Stale:             row.Stale,
	}
	if row.HasSnapshot {
		b.Snapshot = fleetv1.ClusterSnapshot_builder{
			Observed:            timestampOrNil(row.Observed),
			Nodes:               row.Nodes.Int32,
			NodesReady:          row.NodesReady.Int32,
			Pods:                row.Pods.Int32,
			PodAllocatable:      row.PodAllocatable.Int32,
			CpuAllocatableM:     row.CpuAllocatableM.Int64,
			MemoryAllocatableMi: row.MemoryAllocatableMi.Int64,
			LastError:           stringOrNil(row.LastError),
			LastErrorAt:         timestampOrNil(row.LastErrorAt),
		}.Build()
	}
	return b.Build()
}

func organizationSummaryFrom(row *db.OrganizationListRow, clustersUnhealthy int32) *fleetv1.OrganizationSummary {
	return fleetv1.OrganizationSummary_builder{
		Id:                row.ID.String(),
		Name:              row.Name,
		Alias:             row.Alias,
		Created:           timestampOrNil(row.Created),
		Clusters:          row.Clusters,
		ClustersUnhealthy: clustersUnhealthy,
		Nodes:             row.Nodes,
		NodesReady:        row.NodesReady,
		Pods:              row.Pods,
		Namespaces:        row.Namespaces,
	}.Build()
}
