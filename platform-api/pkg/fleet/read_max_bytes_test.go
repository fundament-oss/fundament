package fleet_test

import (
	"log/slog"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/platform-api/pkg/fleet"
	"github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1/fleetv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	srv := fleet.New(slog.New(slog.DiscardHandler), fleet.Config{JWTSecret: []byte("secret")}, &psqldb.DB{})

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), fleetv1connect.FleetServiceGetFleetSummaryProcedure)
	connectlimittest.AssertReflectionRejectsGzipBomb(t, srv.Handler())
}
