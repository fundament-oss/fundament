package registry_test

import (
	"log/slog"
	"testing"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1/registryv1connect"
	"github.com/fundament-oss/fundament/marketplace-registry-api/pkg/registry"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	srv := registry.New(slog.New(slog.DiscardHandler), registry.Config{JWTSecret: []byte("secret")}, &psqldb.DB{}, nil)

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), registryv1connect.PublicationServiceListPluginsProcedure)
	connectlimittest.AssertReflectionRejectsGzipBomb(t, srv.Handler())
}
