package admin_test

import (
	"log/slog"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-admin-api/pkg/admin"
	"github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1/adminv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	srv := admin.New(slog.New(slog.DiscardHandler), admin.Config{JWTSecret: []byte("secret")}, &psqldb.DB{})

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), adminv1connect.ReviewServiceListSubmissionsProcedure)
	connectlimittest.AssertReflectionRejectsGzipBomb(t, srv.Handler())
}
