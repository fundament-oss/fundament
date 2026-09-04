package registry_test

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
	"github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/registry/v1/registryv1connect"
	"github.com/fundament-oss/fundament/marketplace-api/pkg/registry"
)

var testJWTSecret = []byte("registry-test-secret")

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
// can seed freely without colliding.
type testEnv struct {
	dbName       string
	adminPool    *pgxpool.Pool
	registryPool *pgxpool.Pool
}

func registryDSN(dbName string) string {
	return fmt.Sprintf("postgres://fun_marketplace_registry_api@localhost:%d/%s?sslmode=disable", testDBPort, dbName)
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbName := testNameToDBName(t.Name())
	createTestDatabase(t, dbName)

	adminPool, err := pgxpool.New(context.Background(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	// The registry connects as its own role so every assertion runs through the
	// RLS policies rather than around them.
	registryPool, err := pgxpool.New(context.Background(), registryDSN(dbName))
	require.NoError(t, err)
	t.Cleanup(registryPool.Close)

	return &testEnv{dbName: dbName, adminPool: adminPool, registryPool: registryPool}
}

// newClient starts the real server over HTTP. Unlike the catalog's tests, which
// call handler methods directly, these go over the wire: organization scoping
// lives in headers, so a direct call would bypass the thing most worth testing.
// registryClient is the generated client under test.
type registryClient = registryv1connect.PublicationServiceClient

func newClient(t *testing.T, env *testEnv, opts ...connect.ClientOption) registryClient {
	t.Helper()

	database, err := registry.NewDB(context.Background(), slog.Default(), psqldb.Config{URL: registryDSN(env.dbName)})
	require.NoError(t, err)
	t.Cleanup(database.Close)

	server := registry.New(slog.Default(), registry.Config{JWTSecret: testJWTSecret}, database)

	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	return registryv1connect.NewPublicationServiceClient(ts.Client(), ts.URL, opts...)
}

// authFor returns client options presenting the given user as a member of the
// given organizations, acting for the first of them.
func authFor(t *testing.T, userID uuid.UUID, organizationIDs ...uuid.UUID) connect.ClientOption {
	t.Helper()

	token := mintToken(t, userID, organizationIDs...)
	active := ""
	if len(organizationIDs) > 0 {
		active = organizationIDs[0].String()
	}

	return connect.WithInterceptors(connect.UnaryInterceptorFunc(
		func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+token)
				if active != "" {
					req.Header().Set(registry.OrganizationHeader, active)
				}
				return next(ctx, req)
			}
		},
	))
}

func mintToken(t *testing.T, userID uuid.UUID, organizationIDs ...uuid.UUID) string {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{string(auth.TokenTypeUser)},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		OrganizationIDs: organizationIDs,
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testJWTSecret)
	require.NoError(t, err)

	return signed
}

func connectCode(err error) connect.Code {
	return connect.CodeOf(err)
}
