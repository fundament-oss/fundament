package organization

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	commongardener "github.com/fundament-oss/fundament/common/gardener"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// The derivation itself is common/clusterstatus's (TestFromDB); this pins
// the mapping onto the console's API.
func TestClusterStatusFromDB(t *testing.T) {
	t.Parallel()

	shoot := func(status string) pgtype.Text { return pgtype.Text{String: status, Valid: true} }
	unhealthy := pgtype.Text{String: "unhealthy", Valid: true}
	awaiting := pgtype.Text{String: commongardener.MsgShootAwaitingHealth, Valid: true}
	deleted := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	none := pgtype.Text{}

	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING, clusterStatusFromDB(pgtype.Timestamptz{}, none, none, none, false, none, false))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING, clusterStatusFromDB(pgtype.Timestamptz{}, shoot("ready"), awaiting, unhealthy, false, none, true))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING, clusterStatusFromDB(pgtype.Timestamptz{}, shoot("ready"), none, none, false, none, true))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING, clusterStatusFromDB(pgtype.Timestamptz{}, shoot("ready"), none, none, true, none, true))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY, clusterStatusFromDB(pgtype.Timestamptz{}, shoot("ready"), none, unhealthy, false, none, true))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR, clusterStatusFromDB(pgtype.Timestamptz{}, shoot("error"), none, none, false, none, true))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING, clusterStatusFromDB(deleted, none, none, none, false, none, true))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED, clusterStatusFromDB(pgtype.Timestamptz{}, shoot("deleted"), none, none, false, none, true))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_LOST, clusterStatusFromDB(pgtype.Timestamptz{}, shoot("lost"), none, none, false, none, true))
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED, clusterStatusFromDB(pgtype.Timestamptz{}, shoot("odd"), none, none, false, none, true))
}

// A sync row's status_info is an error only while the row is retrying or
// failed; a pending row's status_info names the precondition it waits on.
func TestSyncStateFromRowErrorOnlyForFailures(t *testing.T) {
	t.Parallel()

	text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
	none := pgtype.Text{}

	tests := []struct {
		name      string
		status    pgtype.Text
		info      pgtype.Text
		wantError bool
	}{
		{name: "never synced", status: none, info: none},
		{name: "pending without info", status: text("pending"), info: none},
		{name: "pending on a precondition", status: text("pending"), info: text("parent cluster not synced to Gardener")},
		{name: "completed", status: text("completed"), info: none},
		{name: "retrying", status: text("retrying"), info: text("boom"), wantError: true},
		{name: "failed", status: text("failed"), info: text("boom"), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state := syncStateFromRow(tt.status, 0, tt.info, none, none, pgtype.Timestamptz{}, false)
			assert.Equal(t, tt.wantError, state.HasOutboxError())
			if tt.wantError {
				assert.Equal(t, tt.info.String, state.GetOutboxError())
			}
		})
	}
}
