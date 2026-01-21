package dbversion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AssertLatestVersion returns an error when the database is not migrated to the
// latest version.
func AssertLatestVersion(ctx context.Context, pool *pgxpool.Pool) error {
	var version int
	var dirty bool
	err := pool.QueryRow(ctx,
		"SELECT version, dirty FROM schema_migrations",
	).Scan(&version, &dirty)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no migration version found in schema_migrations table")
		}
		return fmt.Errorf("could not query db version: %w", err)
	}

	if dirty {
		return fmt.Errorf("migrations are dirty at version %d", version)
	}
	if version != LatestVersion {
		return fmt.Errorf(
			"migrations are at version %d, require version %d",
			version,
			LatestVersion,
		)
	}
	return nil
}

// MustAssertLatestVersion panics with when the database is not migrated to the
// latest version.
func MustAssertLatestVersion(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) {
	err := AssertLatestVersion(ctx, pool)
	if err != nil {
		logger.Error("database schema version check failed", "error", err)
		panic(fmt.Sprintf("database schema version check failed: %v", err))
	}
	logger.Debug("database schema version verified", "version", LatestVersion)
}
