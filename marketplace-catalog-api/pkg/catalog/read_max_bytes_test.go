package catalog_test

import (
	"log/slog"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/catalog"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1/catalogv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	srv := catalog.New(slog.New(slog.DiscardHandler), &psqldb.DB{}, nil)

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), catalogv1connect.CatalogServiceListPluginsProcedure)
	connectlimittest.AssertReflectionRejectsGzipBomb(t, srv.Handler())
}
