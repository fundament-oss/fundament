package organization

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func TestNodePoolFromRowStoredStatus(t *testing.T) {
	t.Parallel()
	row := db.TenantNodePool{Name: "nomach", MachineType: "local-medium", AutoscaleMin: 1, AutoscaleMax: 1}

	row.Status = pgtype.Text{String: "waiting", Valid: true}
	row.StatusMessage = pgtype.Text{String: "no machines", Valid: true}
	pool := nodePoolFromRow(&row, nodePoolRuntime{})
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_WAITING_FOR_MACHINES, pool.GetStatus())
	assert.Equal(t, "no machines", pool.GetStatusMessage())

	row.Status = pgtype.Text{String: "progressing", Valid: true}
	row.StatusMessage = pgtype.Text{}
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_PROVISIONING, nodePoolFromRow(&row, nodePoolRuntime{}).GetStatus())

	row.Status = pgtype.Text{String: "error", Valid: true}
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_FAILED, nodePoolFromRow(&row, nodePoolRuntime{}).GetStatus())

	// Ready (or never classified) defers to the metrics runtime.
	row.Status = pgtype.Text{String: "ready", Valid: true}
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_HEALTHY, nodePoolFromRow(&row, nodePoolRuntime{nodes: 2, ready: 2}).GetStatus())
	row.Status = pgtype.Text{}
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, nodePoolFromRow(&row, nodePoolRuntime{}).GetStatus())
}
