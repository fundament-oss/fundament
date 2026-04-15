package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
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
//
// Precondition dependency graph:
//
//	EntityNodePool → parent cluster must have been synced to Gardener at least once
//	    (resolves node_pool_id → cluster_id, checks ClusterHasEverBeenSynced)
//	EntityCluster (sync, not delete) → Gardener project namespace must be ready
//	    (checks EnsureProject returns non-empty namespace)
//
//	Usersync handler (not declared here, query-level gates):
//	    EntityOrgUser → fans out only to clusters where shoot_status = 'ready'
//	    EntityProjectMember → only if cluster shoot_status = 'ready'
//	    EntityCluster (event=ready) → cluster is ready by definition
type Handler struct {
	pool          *pgxpool.Pool
	queries       *db.Queries
	gardener      ShootSyncer
	statusChecker ShootStatusChecker
	logger        *slog.Logger
	cfg           Config

	preconditions map[handler.EntityType][]handler.Precondition
}

func New(pool *pgxpool.Pool, syncer ShootSyncer, statusChecker ShootStatusChecker, logger *slog.Logger, cfg Config) *Handler {
	queries := db.New(pool)

	h := &Handler{
		pool:          pool,
		queries:       queries,
		gardener:      syncer,
		statusChecker: statusChecker,
		logger:        logger.With("handler", "cluster"),
		cfg:           cfg,
		preconditions: make(map[handler.EntityType][]handler.Precondition),
	}

	h.preconditions[handler.EntityNodePool] = []handler.Precondition{
		{
			Description: "parent cluster must have been synced to Gardener at least once",
			Check: func(ctx context.Context, nodePoolID uuid.UUID) error {
				clusterID, err := queries.NodePoolGetClusterID(ctx, db.NodePoolGetClusterIDParams{NodePoolID: nodePoolID})
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return nil // node pool not found — syncNodePool will handle gracefully
					}
					return fmt.Errorf("resolve node_pool_id → cluster_id: %w", err)
				}
				synced, err := queries.ClusterHasEverBeenSynced(ctx, db.ClusterHasEverBeenSyncedParams{
					ClusterID: pgtype.UUID{Bytes: clusterID, Valid: true},
				})
				if err != nil {
					return fmt.Errorf("check cluster ever synced: %w", err)
				}
				if !synced {
					return handler.NewPreconditionError("parent cluster not synced to Gardener")
				}
				return nil
			},
		},
	}

	h.preconditions[handler.EntityCluster] = []handler.Precondition{
		{
			Description: "Gardener project namespace must be ready",
			Check: func(ctx context.Context, clusterID uuid.UUID) error {
				cluster, err := queries.ClusterGetForSync(ctx, db.ClusterGetForSyncParams{ClusterID: clusterID})
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return nil // cluster not found — syncCluster will handle gracefully
					}
					return fmt.Errorf("get cluster for precondition check: %w", err)
				}
				if cluster.Deleted.Valid {
					return nil // delete path skips EnsureProject
				}
				namespace, err := syncer.EnsureProject(ctx, gardener.ProjectName(cluster.OrganizationName), cluster.OrganizationID)
				if err != nil {
					return fmt.Errorf("ensure project: %w", err)
				}
				if namespace == "" {
					return handler.NewPreconditionError("project namespace not ready")
				}
				return nil
			},
		},
	}

	return h
}

// checkPreconditions runs all declared preconditions for the given entity type.
// Returns the first PreconditionError encountered, or nil if all pass.
func (h *Handler) checkPreconditions(ctx context.Context, entityType handler.EntityType, id uuid.UUID) error {
	for _, p := range h.preconditions[entityType] {
		if err := p.Check(ctx, id); err != nil {
			return fmt.Errorf("precondition %q: %w", p.Description, err)
		}
	}
	return nil
}
