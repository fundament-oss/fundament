package storage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/organization-api/pkgs/storage/sqlc/db"
)

type Storage struct {
	pool    *pgxpool.Pool
	Queries *db.Queries
	logger  *slog.Logger
}

func New(ctx context.Context, databaseURL string, logger *slog.Logger) (*Storage, error) {
	logger.Debug("creating database connection pool")

	pool, err := pgxpool.New(ctx, databaseURL)
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
	return &Storage{
		pool:    pool,
		Queries: db.New(pool),
		logger:  logger,
	}, nil
}

func (s *Storage) Close() {
	s.logger.Debug("closing database connection pool")
	s.pool.Close()
}
