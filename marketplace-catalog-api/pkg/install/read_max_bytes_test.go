package install_test

import (
	"log/slog"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/install"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/install/v1/installv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	srv := install.New(slog.New(slog.DiscardHandler), install.Config{JWTSecret: []byte("secret")}, &psqldb.DB{})

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), installv1connect.InstallServiceListPluginsProcedure)
}
