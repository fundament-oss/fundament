package overview_test

import (
	"log/slog"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/platform-api/pkg/overview"
	"github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/platform/v1/platformv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	srv := overview.New(slog.New(slog.DiscardHandler), nil, overview.Config{})

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), platformv1connect.OverviewServiceGetPlatformOverviewProcedure)
}
