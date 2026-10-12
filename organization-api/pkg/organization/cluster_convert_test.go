package organization

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	commongardener "github.com/fundament-oss/fundament/common/gardener"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func TestClusterStatusFromDB(t *testing.T) {
	t.Parallel()

	shoot := func(status string) pgtype.Text { return pgtype.Text{String: status, Valid: true} }
	healthy := pgtype.Text{String: "healthy", Valid: true}
	unhealthy := pgtype.Text{String: "unhealthy", Valid: true}
	active := pgtype.Timestamptz{}
	deleted := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	awaiting := pgtype.Text{String: commongardener.MsgShootAwaitingHealth, Valid: true}

	tests := []struct {
		name     string
		deleted  pgtype.Timestamptz
		status   pgtype.Text
		message  pgtype.Text
		health   pgtype.Text
		updating bool
		want     organizationv1.ClusterStatus
	}{
		{name: "not checked yet", deleted: active, want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "being created", deleted: active, status: shoot("progressing"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "ready", deleted: active, status: shoot("ready"), health: healthy, want: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING},
		{name: "ready, health unknown", deleted: active, status: shoot("ready"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING},
		{name: "ready while an update rolls out", deleted: active, status: shoot("ready"), health: healthy, updating: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING},
		{name: "ready but unhealthy", deleted: active, status: shoot("ready"), health: unhealthy, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY},
		{name: "unhealthy wins over updating", deleted: active, status: shoot("ready"), health: unhealthy, updating: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY},
		{name: "progressing unaffected by health", deleted: active, status: shoot("progressing"), health: unhealthy, want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "failed", deleted: active, status: shoot("error"), want: organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR},
		{name: "deleted while updating", deleted: deleted, status: shoot("ready"), updating: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING},
		{name: "settling after create is starting, not unhealthy", deleted: active, status: shoot("ready"), message: awaiting, health: unhealthy, want: organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING},
		{name: "unhealthy after settling once the message moves on", deleted: active, status: shoot("ready"), message: pgtype.Text{String: "Shoot reconciled but not all conditions healthy", Valid: true}, health: unhealthy, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, clusterStatusFromDB(tt.deleted, tt.status, tt.message, tt.health, tt.updating))
		})
	}
}
