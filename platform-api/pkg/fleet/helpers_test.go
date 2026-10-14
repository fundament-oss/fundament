package fleet_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
	"github.com/fundament-oss/fundament/platform-api/pkg/fleet"
	"github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1/fleetv1connect"
)

var testJWTSecret = []byte("fleet-test-secret")

const testStaleAfter = 45 * time.Minute

// newClient starts the real server over HTTP, connected as the admin role:
// authentication lives in headers, so a direct handler call would bypass the
// thing most worth testing.
func newClient(t *testing.T, env *testpg.Env, opts ...connect.ClientOption) fleetv1connect.FleetServiceClient {
	t.Helper()

	database, err := psqldb.New(context.Background(), slog.Default(), psqldb.Config{URL: env.RoleDSN("fun_platform_admin_api")})
	require.NoError(t, err)
	t.Cleanup(database.Close)

	server := fleet.New(slog.New(slog.DiscardHandler), fleet.Config{
		JWTSecret:  testJWTSecret,
		StaleAfter: testStaleAfter,
	}, database)

	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	return fleetv1connect.NewFleetServiceClient(ts.Client(), ts.URL, opts...)
}

// staffClient is a client presenting a DCIM token.
func staffClient(t *testing.T, env *testpg.Env) fleetv1connect.FleetServiceClient {
	t.Helper()
	return newClient(t, env, bearer(mintToken(t, auth.DCIMIssuer, nil)))
}

func bearer(token string) connect.ClientOption {
	return connect.WithInterceptors(connect.UnaryInterceptorFunc(
		func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+token)
				return next(ctx, req)
			}
		},
	))
}

// mintToken mirrors dcim-authn-api's mintJWT for the DCIM issuer (no audience,
// no organization ids) and authn-api's user token for the console issuer.
func mintToken(t *testing.T, issuer string, audience jwt.ClaimStrings) string {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   uuid.NewString(),
			Audience:  audience,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testJWTSecret)
	require.NoError(t, err)
	return signed
}
