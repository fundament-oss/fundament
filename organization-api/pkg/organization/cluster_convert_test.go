package organization

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func TestClusterStatusFromDB(t *testing.T) {
	t.Parallel()
	ready := pgtype.Text{String: "ready", Valid: true}
	unhealthy := pgtype.Text{String: "unhealthy", Valid: true}
	healthy := pgtype.Text{String: "healthy", Valid: true}

	tests := []struct {
		name    string
		deleted pgtype.Timestamptz
		status  pgtype.Text
		health  pgtype.Text
		pending bool
		want    organizationv1.ClusterStatus
	}{
		{name: "ready healthy settled", status: ready, health: healthy, want: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING},
		{name: "ready unhealthy", status: ready, health: unhealthy, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY},
		{name: "ready with pending pools", status: ready, health: healthy, pending: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING},
		{name: "unhealthy wins over pending", status: ready, health: unhealthy, pending: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY},
		{name: "ready health unknown", status: ready, want: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING},
		{name: "progressing unaffected by pools", status: pgtype.Text{String: "progressing", Valid: true}, pending: true, want: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING},
		{name: "deleted wins", deleted: pgtype.Timestamptz{Valid: true}, status: ready, want: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, clusterStatusFromDB(tt.deleted, tt.status, tt.health, tt.pending))
		})
	}
}
