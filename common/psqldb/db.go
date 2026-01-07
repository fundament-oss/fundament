package psqldb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool   *pgxpool.Pool
	logger *slog.Logger
}

type Option = func(ctx context.Context, config *pgxpool.Config)

func New(ctx context.Context, logger *slog.Logger, databaseURL string, options ...Option) (*DB, error) {
	logger.Debug("creating database connection pool")

	// Parse the database URL into a config
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		logger.Error("failed to parse database URL", "error", err)
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	for _, option := range options {
		option(ctx, config)
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
