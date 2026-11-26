package handler

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	db "github.com/fundament-oss/fundament/authz-worker/pkg/db/gen"
)

// A deleted cluster keeps its tuple until Gardener confirms its Shoot is gone.
func TestClusterAccessEnded(t *testing.T) {
	deleted := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	status := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

	tests := []struct {
		name    string
		cluster db.GetClusterByIDRow
		ended   bool
	}{
		{name: "live", cluster: db.GetClusterByIDRow{ShootStatus: status("ready")}},
		{name: "live and stopped", cluster: db.GetClusterByIDRow{ShootStatus: status("deleted")}},
		{name: "deleted, no status yet", cluster: db.GetClusterByIDRow{Deleted: deleted}},
		{name: "deleted, being torn down", cluster: db.GetClusterByIDRow{Deleted: deleted, ShootStatus: status("deleting")}},
		{name: "deleted, confirmed gone", cluster: db.GetClusterByIDRow{Deleted: deleted, ShootStatus: status("deleted")}, ended: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.ended, clusterAccessEnded(&tt.cluster))
		})
	}
}
