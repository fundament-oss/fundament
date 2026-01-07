package psqldb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool   *pgxpool.Pool
	logger *slog.Logger
}

func New(ctx context.Context, logger *slog.Logger, databaseURL string) (*DB, error) {
	logger.Debug("creating database connection pool")

	// Parse the database URL into a config
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		logger.Error("failed to parse database URL", "error", err)
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	// Set up AfterRelease callback to reset session-level settings
	// This ensures tenant isolation when connections are reused from the pool
	config.AfterRelease = func(conn *pgx.Conn) bool {
		// Reset any session-level settings before returning to pool
		_, err := conn.Exec(context.Background(), "RESET app.current_tenant_id")
		if err != nil {
			logger.Warn("failed to reset tenant context on connection release, destroying connection", "error", err)
			return false // Destroy connection to prevent tenant data leakage
		}
		return true // Keep connection in pool
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		logger.Error("failed to create connection pool", "error", err)
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	logger.Debug("pinging database")
	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping database", "error", err)
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	logger.Info("database connection established")
	return &DB{
		Pool:   pool,
		logger: logger,
	}, nil
}

func (s *DB) Close() {
	s.logger.Debug("closing database connection pool")
	s.Pool.Close()
}
