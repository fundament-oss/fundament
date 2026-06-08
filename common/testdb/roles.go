// Package testdb provides shared helpers for setting up the test PostgreSQL
// instance used by package-level tests across fundament.
package testdb

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Role represents a database role that must exist before running tests.
type Role struct {
	Name      string
	BypassRLS bool
}

// Roles is the canonical list of database roles used by the test setup of
// every package that talks to PostgreSQL. Add new fundament roles here so
// they get picked up by all test setups at once.
var Roles = []Role{
	{Name: "fun_authn_api"},
	{Name: "fun_fundament_api"},
	{Name: "fun_operator", BypassRLS: true},
	{Name: "fun_owner"},
	{Name: "fun_authz", BypassRLS: true},
	{Name: "fun_cluster_worker"},
	{Name: "fun_authz_worker", BypassRLS: true},
	{Name: "fun_dcim_api"},
}

// CreateRoles ensures every role in [Roles] exists with the configured
// BYPASSRLS setting. It assumes the database is reachable via trust auth,
// which is the default for tests that use the embedded postgres setup.
// Intended to be called from TestMain; it terminates the process on failure.
func CreateRoles(ctx context.Context, pool *pgxpool.Pool) {
	for _, role := range Roles {
		_, err := pool.Exec(ctx, fmt.Sprintf(`DO $$ BEGIN CREATE ROLE %s WITH LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$`, role.Name))
		if err != nil {
			log.Fatalf("failed to create role %s: %v", role.Name, err)
		}

		bypassrls := "NOBYPASSRLS"
		if role.BypassRLS {
			bypassrls = "BYPASSRLS"
		}

		_, err = pool.Exec(ctx, fmt.Sprintf(`ALTER ROLE %s %s`, role.Name, bypassrls))
		if err != nil {
			log.Fatalf("failed to alter role %s: %v", role.Name, err)
		}
	}
}
