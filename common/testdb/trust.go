package testdb

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UseGlobalTrustAuth reconfigures the embedded-postgres instance to accept
// connections without any credential check. The embedded-postgres library
// hardcodes `initdb -A password`, so `pg_hba.conf` requires password auth for
// every connection, while the roles created by [CreateRoles] have no password.
// Intended to be called from TestMain before [CreateRoles]; it terminates the
// process on failure.
func UseGlobalTrustAuth(dataDir string, pool *pgxpool.Pool) {
	pgHBAPath := filepath.Join(dataDir, "pg_hba.conf")
	content, err := os.ReadFile(pgHBAPath) //nolint:gosec // test helper, paths are not user-controlled
	if err != nil {
		log.Fatalf("failed to read pg_hba.conf: %v", err)
	}
	updated := strings.ReplaceAll(string(content), " password\n", " trust\n")
	if err := os.WriteFile(pgHBAPath, []byte(updated), 0o600); err != nil { //nolint:gosec // test helper
		log.Fatalf("failed to write pg_hba.conf: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "SELECT pg_reload_conf()"); err != nil {
		log.Fatalf("failed to reload pg_hba.conf: %v", err)
	}
}
