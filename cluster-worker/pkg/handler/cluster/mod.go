package cluster

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// ShootSyncer provides the Gardener operations needed by the sync path.
type ShootSyncer interface {
	EnsureProject(ctx context.Context, projectName string, orgID uuid.UUID) (namespace string, err error)
	ApplyShoot(ctx context.Context, cluster *gardener.ClusterToSync) error
	DeleteShootByClusterID(ctx context.Context, clusterID uuid.UUID) error
	ListShoots(ctx context.Context) ([]gardener.ShootInfo, error)
}

// ShootStatusChecker provides the Gardener operations needed by the status path.
type ShootStatusChecker interface {
	GetShootStatus(ctx context.Context, cluster *gardener.ClusterToSync) (*gardener.ShootStatus, error)
	RequestAdminKubeconfig(ctx context.Context, clusterID uuid.UUID, expirationSeconds int64) (*gardener.AdminKubeconfig, error)
}

// Config holds handler-specific configuration.
type Config struct {
	StatusBatchSize int32 `env:"STATUS_BATCH_SIZE" envDefault:"50"`
	MaxRetries      int32 `env:"MAX_RETRIES" envDefault:"10"`
}

// Handler manages cluster lifecycle in Gardener (sync, status, orphan cleanup).
type Handler struct {
	pool          *pgxpool.Pool
	queries       *db.Queries
	gardener      ShootSyncer
	statusChecker ShootStatusChecker
	logger        *slog.Logger
	cfg           Config
}

func New(pool *pgxpool.Pool, syncer ShootSyncer, statusChecker ShootStatusChecker, logger *slog.Logger, cfg Config) *Handler {
	return &Handler{
		pool:          pool,
		queries:       db.New(pool),
		gardener:      syncer,
		statusChecker: statusChecker,
		logger:        logger.With("handler", "cluster"),
		cfg:           cfg,
	}
}
