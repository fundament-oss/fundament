package organization

import (
	"fmt"

	"github.com/fundament-oss/fundament/common/dbconst"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func clusterStatusFromDB(status dbconst.ClusterStatus) organizationv1.ClusterStatus {
	switch status {
	case dbconst.ClusterStatus_Provisioning:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING
	case dbconst.ClusterStatus_Starting:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING
	case dbconst.ClusterStatus_Running:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING
	case dbconst.ClusterStatus_Upgrading:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING
	case dbconst.ClusterStatus_Error:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR
	case dbconst.ClusterStatus_Stopping:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING
	case dbconst.ClusterStatus_Stopped:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	case dbconst.ClusterStatus_Unspecified:
		return organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	default:
		panic(fmt.Sprintf("unknown cluster status from db: %s", status))
	}
}
