package fleet

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

// clusterStatus reads shoot_status the way the console does
// (organization-api's clusterStatusFromDB). The column has no check
// constraint, so an unknown value is reported as unspecified rather than
// failing the whole list.
func clusterStatus(shootStatus pgtype.Text, updating bool) fleetv1.ClusterStatus {
	if !shootStatus.Valid {
		return fleetv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	}
	switch shootStatus.String {
	case "pending", "progressing":
		return fleetv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case "ready":
		if updating {
			return fleetv1.ClusterStatus_CLUSTER_STATUS_UPDATING
		}
		return fleetv1.ClusterStatus_CLUSTER_STATUS_RUNNING
	case "error":
		return fleetv1.ClusterStatus_CLUSTER_STATUS_ERROR
	case "deleting":
		return fleetv1.ClusterStatus_CLUSTER_STATUS_DELETING
	case "deleted":
		return fleetv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	default:
		return fleetv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	}
}

// clusterHealth reads shoot_health, which clusters_ck_shoot_health limits to
// healthy and unhealthy; NULL means not known.
func clusterHealth(shootHealth pgtype.Text) fleetv1.ClusterHealth {
	if !shootHealth.Valid {
		return fleetv1.ClusterHealth_CLUSTER_HEALTH_UNSPECIFIED
	}
	switch shootHealth.String {
	case "healthy":
		return fleetv1.ClusterHealth_CLUSTER_HEALTH_HEALTHY
	case "unhealthy":
		return fleetv1.ClusterHealth_CLUSTER_HEALTH_UNHEALTHY
	default:
		panic(fmt.Sprintf("unhandled shoot_health %q", shootHealth.String))
	}
}

// unhealthy is the rule behind clusters_unhealthy and the UNHEALTHY filter:
// Gardener calls it unhealthy, or it is in error.
func unhealthy(row *db.ClusterListRow) bool {
	return clusterHealth(row.ShootHealth) == fleetv1.ClusterHealth_CLUSTER_HEALTH_UNHEALTHY ||
		clusterStatus(row.ShootStatus, row.ShootUpdating) == fleetv1.ClusterStatus_CLUSTER_STATUS_ERROR
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
		Status:            clusterStatus(row.ShootStatus, row.ShootUpdating),
		Health:            clusterHealth(row.ShootHealth),
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

func organizationSummaryFrom(row *db.OrganizationListRow) *fleetv1.OrganizationSummary {
	return fleetv1.OrganizationSummary_builder{
		Id:                row.ID.String(),
		Name:              row.Name,
		Alias:             row.Alias,
		Created:           timestampOrNil(row.Created),
		Clusters:          row.Clusters,
		ClustersUnhealthy: row.ClustersUnhealthy,
		Nodes:             row.Nodes,
		NodesReady:        row.NodesReady,
		Pods:              row.Pods,
		Namespaces:        row.Namespaces,
	}.Build()
}
