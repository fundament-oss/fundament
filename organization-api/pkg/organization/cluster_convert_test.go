package organization

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func TestClusterStatusFromDB(t *testing.T) {
	t.Parallel()

	shoot := func(status string) pgtype.Text { return pgtype.Text{String: status, Valid: true} }
	sync := func(status string) pgtype.Text { return pgtype.Text{String: status, Valid: true} }
	active := pgtype.Timestamptz{}
	deleted := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	tests := []struct {
		name     string
		deleted  pgtype.Timestamptz
		status   pgtype.Text
		updating bool
		sync     pgtype.Text
		want     organizationv1.ClusterStatus
	}{
		{name: "not checked yet", deleted: active, want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "sync pending", deleted: active, sync: sync("pending"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "sync retrying", deleted: active, sync: sync("retrying"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "sync failed before the shoot existed", deleted: active, sync: sync("failed"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR},
		{name: "sync failed while the shoot is lost", deleted: active, status: shoot("pending"), sync: sync("failed"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR},
		{name: "being created", deleted: active, status: shoot("progressing"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "ready", deleted: active, status: shoot("ready"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING},
		{name: "ready, a later sync failed", deleted: active, status: shoot("ready"), sync: sync("failed"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING},
		{name: "ready while an update rolls out", deleted: active, status: shoot("ready"), updating: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING},
		{name: "failed", deleted: active, status: shoot("error"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR},
		{name: "deleted while updating", deleted: deleted, status: shoot("ready"), updating: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING},
		{name: "deleted after its sync failed", deleted: deleted, sync: sync("failed"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, clusterStatusFromDB(tt.deleted, tt.status, tt.updating, tt.sync))
		})
	}
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
			state := syncStateFromRow(tt.status, 0, tt.info, none, none, none, none, pgtype.Timestamptz{})
			assert.Equal(t, tt.wantError, state.HasOutboxError())
			if tt.wantError {
				assert.Equal(t, tt.info.String, state.GetOutboxError())
			}
		})
	}
}
