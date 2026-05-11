package dcim_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/dcim-api/pkg/dcim"
)

const (
	validUUID   = "550e8400-e29b-41d4-a716-446655440000"
	invalidUUID = "not-a-uuid"
)

type testEnv struct {
	server    *httptest.Server
	adminPool *pgxpool.Pool
}

type apiOptions struct {
	t testing.TB
}

type APIOption func(*apiOptions)

func newTestAPI(t *testing.T, options ...APIOption) *testEnv {
	t.Helper()

	opts := apiOptions{t: t}
	for _, option := range options {
		option(&opts)
	}

	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	testDB, adminPool := createTestDB(t, testLogger)

	srv := dcim.New(testLogger, testDB)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &testEnv{
		server:    ts,
		adminPool: adminPool,
	}
}

func createTestDB(t *testing.T, logger *slog.Logger) (*psqldb.DB, *pgxpool.Pool) {
	t.Helper()

	name := testNameToDBName(t.Name())
	createTestDatabase(t, name)

	testDB, err := psqldb.New(t.Context(), logger, psqldb.Config{
		URL: fmt.Sprintf("postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, name),
	})
	require.NoError(t, err)
	t.Cleanup(testDB.Close)

	adminPool, err := pgxpool.New(t.Context(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable",
		testDBPort, name,
	))
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	return testDB, adminPool
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

func testNameToDBName(testName string) string {
	name := strings.ToLower(testName)
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")

	if len(name) > 63 {
		name = name[:63]
	}

	return name
}

func requireCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, want, connect.CodeOf(err))
}
