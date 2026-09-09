package admin_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-admin-api/pkg/admin"
	"github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1/adminv1connect"
)

var testJWTSecret = []byte("admin-test-secret")

func createTestDatabase(t *testing.T, name string) {
	t.Helper()

	adminURL := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", testDBPort)

	adminPool, err := pgxpool.New(context.Background(), adminURL)
	require.NoError(t, err)
	defer adminPool.Close()

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	require.NoError(t, err)
}

func testNameToDBName(testName string) string {
	name := strings.ToLower(testName)
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")

	if len(name) > 63 {
		name = name[:63]
	}

	return name
}

// Each test gets its own database cloned from the fundament template, so tests
// can seed freely without colliding. superPool connects as postgres and seeds
// around RLS; servicePool connects as the service's own role.
type testEnv struct {
	dbName      string
	superPool   *pgxpool.Pool
	servicePool *pgxpool.Pool
}

func serviceDSN(dbName string) string {
	return fmt.Sprintf("postgres://fun_marketplace_admin_api@localhost:%d/%s?sslmode=disable", testDBPort, dbName)
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbName := testNameToDBName(t.Name())
	createTestDatabase(t, dbName)

	superPool, err := pgxpool.New(context.Background(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(superPool.Close)

	// The service connects as its own role so every assertion runs through the
	// grants and RLS policies rather than around them.
	servicePool, err := pgxpool.New(context.Background(), serviceDSN(dbName))
	require.NoError(t, err)
	t.Cleanup(servicePool.Close)

	return &testEnv{dbName: dbName, superPool: superPool, servicePool: servicePool}
}

// newClient starts the real server over HTTP: authentication lives in headers,
// so a direct handler call would bypass the thing most worth testing.
type reviewClient = adminv1connect.ReviewServiceClient

func newClient(t *testing.T, env *testEnv, opts ...connect.ClientOption) reviewClient {
	t.Helper()

	database, err := psqldb.New(context.Background(), slog.Default(), psqldb.Config{URL: serviceDSN(env.dbName)})
	require.NoError(t, err)
	t.Cleanup(database.Close)

	server := admin.New(slog.Default(), admin.Config{JWTSecret: testJWTSecret}, database)

	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	return adminv1connect.NewReviewServiceClient(ts.Client(), ts.URL, opts...)
}

// authFor returns a client option presenting the given user as the reviewer,
// carrying a DCIM token: reviewers are staff (dcim.users), not console users.
// No organization header: no RPC on this surface is organization-scoped.
func authFor(t *testing.T, userID uuid.UUID) connect.ClientOption {
	t.Helper()

	token := mintToken(t, userID)

	return connect.WithInterceptors(connect.UnaryInterceptorFunc(
		func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+token)
				return next(ctx, req)
			}
		},
	))
}

// authWithConsoleToken presents a console UserToken — the customer-facing
// credential the admin surface must refuse by issuer.
func authWithConsoleToken(t *testing.T, userID uuid.UUID) connect.ClientOption {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testJWTSecret)
	require.NoError(t, err)

	return connect.WithInterceptors(connect.UnaryInterceptorFunc(
		func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+signed)
				return next(ctx, req)
			}
		},
	))
}

func mintToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()

	// Mirrors dcim-authn-api's mintJWT: DCIM tokens carry no audience and no
	// organization ids.
	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.DCIMIssuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testJWTSecret)
	require.NoError(t, err)

	return signed
}
