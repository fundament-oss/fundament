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
	active := pgtype.Timestamptz{}
	deleted := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	tests := []struct {
		name     string
		deleted  pgtype.Timestamptz
		status   pgtype.Text
		updating bool
		want     organizationv1.ClusterStatus
	}{
		{name: "not checked yet", deleted: active, want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "being created", deleted: active, status: shoot("progressing"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "ready", deleted: active, status: shoot("ready"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING},
		{name: "ready while an update rolls out", deleted: active, status: shoot("ready"), updating: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING},
		{name: "failed", deleted: active, status: shoot("error"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR},
		{name: "deleted while updating", deleted: deleted, status: shoot("ready"), updating: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, clusterStatusFromDB(tt.deleted, tt.status, tt.updating))
		})
	}
}
