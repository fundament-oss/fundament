package psqldb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	URL string `env:"DATABASE_URL,required,notEmpty"`
}

type DB struct {
	Pool   *pgxpool.Pool
	logger *slog.Logger
}

type Option = func(ctx context.Context, config *pgxpool.Config)

// New is NewLazy followed by a connectivity check, so a misconfigured primary
// pool fails at startup rather than on the first query.
func New(ctx context.Context, logger *slog.Logger, cfg Config, options ...Option) (*DB, error) {
	db, err := NewLazy(ctx, logger, cfg, options...)
	if err != nil {
		return nil, err
	}

	logger.Debug("pinging database")
	if err := db.Pool.Ping(ctx); err != nil {
		logger.Error("failed to ping database", "error", err)
		db.Pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	logger.Debug("database connection established")
	return db, nil
}

func (s *DB) Close() {
	s.logger.Debug("closing database connection pool")
	s.Pool.Close()
}

// NewLazy is New without the connectivity check: the pool is configured but
// no connection is made until the first query. For a secondary pool whose
// backing role or grants may land after the process starts (a chart upgrade
// rolls the Deployment while the migration Job is still running), so that the
// primary surface keeps serving while the secondary reports errors per call.
func NewLazy(ctx context.Context, logger *slog.Logger, cfg Config, options ...Option) (*DB, error) {
	logger.Debug("creating database connection pool")

	pgxcfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		logger.Error("failed to parse database URL", "error", err)
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	for _, option := range options {
		option(ctx, pgxcfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxcfg)
	if err != nil {
		logger.Error("failed to create connection pool", "error", err)
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	return &DB{
		Pool:   pool,
		logger: logger,
	}, nil
}
