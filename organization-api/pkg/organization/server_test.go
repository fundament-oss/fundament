package organization_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/organization-api/pkg/clock"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
)

type testEnv struct {
	server    *httptest.Server
	jwtSecret []byte
	orgs      map[uuid.UUID]string
	users     map[uuid.UUID]testUser
}

type testUser struct {
	Name        string
	Email       string
	ExternalRef string
	OrgIDs      []uuid.UUID
}

type APIOptions struct {
	T             testing.TB
	Organizations map[uuid.UUID]string
	Users         map[uuid.UUID]testUser
	Clock         clock.Clock
}

type APIOption func(*APIOptions)

func WithOrganization(id uuid.UUID, name string) APIOption {
	return func(o *APIOptions) {
		o.Organizations[id] = name
	}
}

func WithUser(id uuid.UUID, name, email, externalRef string, orgIDs []uuid.UUID) APIOption {
	return func(o *APIOptions) {
		_, exists := o.Users[id]
		if exists {
			o.T.Fatalf("WithUser: duplicate user ID %q", id)
		}

		o.Users[id] = testUser{Name: name, Email: email, OrgIDs: orgIDs, ExternalRef: externalRef}
	}
}

func WithClock(c clock.Clock) APIOption {
	return func(o *APIOptions) {
		o.Clock = c
	}
}

func newTestAPI(t *testing.T, options ...APIOption) *testEnv {
	opts := APIOptions{
		T:             t,
		Organizations: make(map[uuid.UUID]string),
		Users:         make(map[uuid.UUID]testUser),
	}
	for _, option := range options {
		option(&opts)
	}

	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	testDb := createTestDB(t)

	jwtSecret := []byte(uuid.New().String())

	organizationCfg := &organization.Config{
		JWTSecret:          jwtSecret,
		CORSAllowedOrigins: []string{"*"},
		Clock:              opts.Clock,
	}

	organizationServer, err := organization.New(testLogger, organizationCfg, testDb, nil)
	require.NoError(t, err)

	ts := httptest.NewServer(organizationServer.Handler())
	t.Cleanup(ts.Close)

	for id, name := range opts.Organizations {
		_, err = testDb.Pool.Exec(t.Context(),
			"INSERT INTO tenant.organizations (id, name) VALUES ($1, $2)",
			id, name,
		)
		require.NoError(t, err)
	}

	for id, user := range opts.Users {
		_, err = testDb.Pool.Exec(t.Context(),
			"INSERT INTO tenant.users (id, name, external_ref, email) VALUES ($1, $2, $3, $4)",
			id, user.Name, user.ExternalRef, user.Email,
		)
		require.NoError(t, err)

		for _, orgID := range user.OrgIDs {
			_, err = testDb.Pool.Exec(t.Context(),
				"INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status) VALUES ($1, $2, 'admin', 'accepted')",
				orgID, id,
			)
			require.NoError(t, err)
		}

	}

	return &testEnv{
		server:    ts,
		jwtSecret: jwtSecret,
		orgs:      opts.Organizations,
		users:     opts.Users,
	}
}

func createTestDB(t *testing.T) *psqldb.DB {
	t.Helper()

	name := testNameToDbName(t.Name())
	createTestDatabase(t, name)

	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	dbCfg := psqldb.Config{
		URL: fmt.Sprintf("postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, name),
	}
	testDb, err := psqldb.New(t.Context(), testLogger, dbCfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		testDb.Close()
	})

	return testDb
}

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

func testNameToDbName(testName string) string {
	name := strings.ToLower(testName)
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")

	if len(name) > 63 {
		name = name[:63]
	}

	return name
}

func (e *testEnv) createAuthnToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()

	user, ok := e.users[userID]
	require.True(t, ok, "user %s not found in test env", userID)

	now := time.Now()

	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "fundament-authn-api",
			Subject:   "external-" + userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		UserID:          userID,
		OrganizationIDs: user.OrgIDs,
		Name:            user.Name,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(e.jwtSecret)
	require.NoError(t, err)

	return signed
}
