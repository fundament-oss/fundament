package organization_test

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/connectlimit/connectlimittest"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

func TestRejectsGzipBombBeforeAuth(t *testing.T) {
	secret := []byte("secret")
	srv, err := organization.New(slog.New(slog.DiscardHandler), &organization.Config{JWTSecret: secret}, &psqldb.DB{}, nil, nil)
	require.NoError(t, err)

	connectlimittest.AssertRejectsGzipBomb(t, srv.Handler(), organizationv1connect.OrganizationServiceListOrganizationsProcedure)

	// The streaming RPCs authenticate before reading, so the walk carries a user token.
	orgID := uuid.New()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   uuid.NewString(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		OrganizationIDs: []uuid.UUID{orgID},
	}).SignedString(secret)
	require.NoError(t, err)

	connectlimittest.AssertEveryProcedureRejectsGzipBombWithHeader(t, srv.Handler(), http.Header{
		"Authorization":         {"Bearer " + token},
		auth.OrganizationHeader: {orgID.String()},
	})
}
