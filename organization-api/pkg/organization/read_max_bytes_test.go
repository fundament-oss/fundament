package organization_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	srv, err := organization.New(slog.New(slog.DiscardHandler), &organization.Config{JWTSecret: []byte("secret")}, &psqldb.DB{}, nil, nil)
	require.NoError(t, err)

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), organizationv1connect.OrganizationServiceListOrganizationsProcedure)
	connectlimittest.AssertReflectionRejectsGzipBomb(t, srv.Handler())
}
