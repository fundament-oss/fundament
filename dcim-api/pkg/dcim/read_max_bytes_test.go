package dcim_test

import (
	"log/slog"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/dcim-api/pkg/dcim"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	srv := dcim.New(slog.New(slog.DiscardHandler), &psqldb.DB{}, []byte("secret"))

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), dcimv1connect.SiteServiceListSitesProcedure)
	connectlimittest.AssertReflectionRejectsGzipBomb(t, srv.Handler())
}
