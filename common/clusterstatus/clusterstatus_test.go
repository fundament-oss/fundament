package clusterstatus_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/common/clusterstatus"
	commongardener "github.com/fundament-oss/fundament/common/gardener"
)

func TestFromDB(t *testing.T) {
	t.Parallel()

	shoot := func(status string) pgtype.Text { return pgtype.Text{String: status, Valid: true} }
	healthy := pgtype.Text{String: "healthy", Valid: true}
	unhealthy := pgtype.Text{String: "unhealthy", Valid: true}
	sync := func(status string) pgtype.Text { return pgtype.Text{String: status, Valid: true} }
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
		sync     pgtype.Text
		reached  bool
		want     clusterstatus.Status
	}{
		{name: "not checked yet", deleted: active, want: clusterstatus.Provisioning},
		{name: "sync pending", deleted: active, sync: sync("pending"), want: clusterstatus.Provisioning},
		{name: "sync retrying", deleted: active, sync: sync("retrying"), want: clusterstatus.Provisioning},
		{name: "sync failed before the shoot existed", deleted: active, sync: sync("failed"), want: clusterstatus.Error},
		{name: "sync failed while the shoot is lost", deleted: active, status: shoot("pending"), sync: sync("failed"), reached: true, want: clusterstatus.Error},
		{name: "update rejected before the first status was recorded", deleted: active, sync: sync("failed"), reached: true, want: clusterstatus.Provisioning},
		{name: "being created", deleted: active, status: shoot("progressing"), want: clusterstatus.Provisioning},
		{name: "ready", deleted: active, status: shoot("ready"), health: healthy, want: clusterstatus.Running},
		{name: "ready, health unknown", deleted: active, status: shoot("ready"), want: clusterstatus.Running},
		{name: "ready while an update rolls out", deleted: active, status: shoot("ready"), health: healthy, updating: true, want: clusterstatus.Upgrading},
		{name: "ready but unhealthy", deleted: active, status: shoot("ready"), health: unhealthy, want: clusterstatus.Unhealthy},
		{name: "unhealthy wins over updating", deleted: active, status: shoot("ready"), health: unhealthy, updating: true, want: clusterstatus.Unhealthy},
		{name: "progressing unaffected by health", deleted: active, status: shoot("progressing"), health: unhealthy, want: clusterstatus.Provisioning},
		{name: "failed", deleted: active, status: shoot("error"), want: clusterstatus.Error},
		{name: "shoot gone from Gardener", deleted: active, status: shoot("lost"), want: clusterstatus.Lost},
		{name: "deleted while updating", deleted: deleted, status: shoot("ready"), updating: true, want: clusterstatus.Deleting},
		{name: "settling after create is starting, not unhealthy", deleted: active, status: shoot("ready"), message: awaiting, health: unhealthy, want: clusterstatus.Starting},
		{name: "unhealthy after settling once the message moves on", deleted: active, status: shoot("ready"), message: pgtype.Text{String: "Shoot reconciled but not all conditions healthy", Valid: true}, health: unhealthy, want: clusterstatus.Unhealthy},
		{name: "ready, a later sync failed", deleted: active, status: shoot("ready"), sync: sync("failed"), want: clusterstatus.Running},
		{name: "deleted after its sync failed", deleted: deleted, sync: sync("failed"), want: clusterstatus.Deleting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, clusterstatus.FromDB(clusterstatus.Input{
				Deleted:         tt.deleted,
				ShootStatus:     tt.status,
				ShootMessage:    tt.message,
				ShootHealth:     tt.health,
				Updating:        tt.updating,
				OutboxStatus:    tt.sync,
				ReachedGardener: tt.reached,
			}))
		})
	}
}

func TestNeedsAttention(t *testing.T) {
	t.Parallel()

	for status, want := range map[clusterstatus.Status]bool{
		clusterstatus.Unspecified:  false,
		clusterstatus.Provisioning: false,
		clusterstatus.Starting:     false,
		clusterstatus.Running:      false,
		clusterstatus.Upgrading:    false,
		clusterstatus.Unhealthy:    true,
		clusterstatus.Error:        true,
		clusterstatus.Deleting:     false,
		clusterstatus.Stopped:      false,
		clusterstatus.Lost:         true,
	} {
		assert.Equal(t, want, status.NeedsAttention(), status)
	}
}
