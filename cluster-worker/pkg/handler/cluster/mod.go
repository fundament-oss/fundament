package cluster

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// Config holds handler-specific configuration.
type Config struct {
	StatusBatchSize int32 `env:"STATUS_BATCH_SIZE" envDefault:"50"`
	MaxRetries      int32 `env:"MAX_RETRIES" envDefault:"10"`
}

// Handler manages cluster lifecycle in Gardener (sync, status, orphan cleanup).
type Handler struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	gardener gardener.Client
	logger   *slog.Logger
	cfg      Config
}

func New(pool *pgxpool.Pool, gardenerClient gardener.Client, logger *slog.Logger, cfg Config) *Handler {
	return &Handler{
		pool:     pool,
		queries:  db.New(pool),
		gardener: gardenerClient,
		logger:   logger.With("handler", "cluster"),
		cfg:      cfg,
	}
}
