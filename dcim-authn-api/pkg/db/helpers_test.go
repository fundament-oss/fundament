package db_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	db "github.com/fundament-oss/fundament/dcim-authn-api/pkg/db/gen"
)

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
// around the grants; queries runs through servicePool, which connects as
// fun_dcim_authn_api — the role the migration's grants fence in.
type testEnv struct {
	superPool   *pgxpool.Pool
	servicePool *pgxpool.Pool
	queries     *db.Queries
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbName := testNameToDBName(t.Name())
	createTestDatabase(t, dbName)

	superPool, err := pgxpool.New(context.Background(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(superPool.Close)

	servicePool, err := pgxpool.New(context.Background(), fmt.Sprintf(
		"postgres://fun_dcim_authn_api@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(servicePool.Close)

	return &testEnv{superPool: superPool, servicePool: servicePool, queries: db.New(servicePool)}
}
