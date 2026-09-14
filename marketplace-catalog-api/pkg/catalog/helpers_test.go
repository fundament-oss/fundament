package catalog_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
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
// can seed freely without colliding.
type testEnv struct {
	dbName      string
	adminPool   *pgxpool.Pool
	catalogPool *pgxpool.Pool
}

func catalogDSN(dbName string) string {
	return fmt.Sprintf("postgres://fun_marketplace_catalog_api@localhost:%d/%s?sslmode=disable", testDBPort, dbName)
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbName := testNameToDBName(t.Name())
	createTestDatabase(t, dbName)

	adminPool, err := pgxpool.New(context.Background(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	// The catalog connects as its own role so every assertion runs through the
	// RLS policies rather than around them.
	catalogPool, err := pgxpool.New(context.Background(), catalogDSN(dbName))
	require.NoError(t, err)
	t.Cleanup(catalogPool.Close)

	return &testEnv{dbName: dbName, adminPool: adminPool, catalogPool: catalogPool}
}
