package idempotency

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/common/idempotency/db/gen"
)

// CachedResponse holds a previously stored response.
type CachedResponse struct {
	Procedure     string
	RequestHash   []byte
	ResponseBytes []byte // nil when reservation is in-progress
	ResourceID    uuid.UUID
}

// ReserveParams holds parameters for reserving an idempotency key.
type ReserveParams struct {
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
	Procedure      string
	RequestHash    []byte
}

// CompleteParams holds parameters for completing an idempotency key reservation.
type CompleteParams struct {
	IdempotencyKey uuid.UUID
	UserID         uuid.UUID
	ResponseBytes  []byte
	ResourceType   ResourceType
	ResourceID     uuid.UUID
}

// Store provides database operations for idempotency keys.
type Store struct {
	queries *db.Queries
	cfg     Config
	logger  *slog.Logger
}

// NewStore creates a new idempotency Store.
func NewStore(pool db.DBTX, cfg Config, logger *slog.Logger) *Store {
	return &Store{queries: db.New(pool), cfg: cfg, logger: logger}
}

// Lookup retrieves a cached response by idempotency key.
func (s *Store) Lookup(ctx context.Context, key, userID uuid.UUID) (*CachedResponse, error) {
	s.logger.DebugContext(ctx, "looking up idempotency key",
		"idempotency_key", key,
		"user_id", userID,
	)

	row, err := s.queries.IdempotencyKeyLookup(ctx, db.IdempotencyKeyLookupParams{
		IdempotencyKey: key,
		UserID:         userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.DebugContext(ctx, "idempotency key not found",
				"idempotency_key", key,
			)
			return nil, nil
		}
		return nil, fmt.Errorf("lookup idempotency key: %w", err)
	}

	s.logger.DebugContext(ctx, "idempotency key found",
		"idempotency_key", key,
		"procedure", row.Procedure,
		"resource_id", row.ResourceID,
	)

	return &CachedResponse{
		Procedure:     row.Procedure,
		RequestHash:   row.RequestHash,
		ResponseBytes: row.ResponseBytes,
		ResourceID:    uuid.UUID(row.ResourceID.Bytes),
	}, nil
}

// Reserve inserts a reservation for an idempotency key (without response data).
// Returns true if the reservation was created, false if a conflict occurred.
func (s *Store) Reserve(ctx context.Context, params ReserveParams) (bool, error) {
	rows, err := s.queries.IdempotencyKeyReserve(ctx, db.IdempotencyKeyReserveParams{
		IdempotencyKey: params.IdempotencyKey,
		UserID:         params.UserID,
		Procedure:      params.Procedure,
		RequestHash:    params.RequestHash,
		Expires: pgtype.Timestamptz{
			Time:  time.Now().Add(s.cfg.ttl()),
			Valid: true,
		},
	})
	if err != nil {
		return false, fmt.Errorf("reserve idempotency key: %w", err)
	}

	reserved := rows == 1
	s.logger.DebugContext(ctx, "idempotency key reserve",
		"idempotency_key", params.IdempotencyKey,
		"reserved", reserved,
	)

	return reserved, nil
}

// Complete updates a reservation with the response data and resource FK.
// Only succeeds if response_bytes IS NULL (i.e. reservation not yet completed).
func (s *Store) Complete(ctx context.Context, params *CompleteParams) error {
	arg := db.IdempotencyKeyCompleteParams{
		IdempotencyKey: params.IdempotencyKey,
		UserID:         params.UserID,
		ResponseBytes:  params.ResponseBytes,
	}

	resourceUUID := pgtype.UUID{Bytes: params.ResourceID, Valid: true}
	switch params.ResourceType {
	case ResourceProject:
		arg.ProjectID = resourceUUID
	case ResourceProjectMember:
		arg.ProjectMemberID = resourceUUID
	case ResourceCluster:
		arg.ClusterID = resourceUUID
	case ResourceNodePool:
		arg.NodePoolID = resourceUUID
	case ResourceNamespace:
		arg.NamespaceID = resourceUUID
	case ResourceAPIKey:
		arg.ApiKeyID = resourceUUID
	case ResourceOrganizationUser:
		arg.OrganizationUserID = resourceUUID
	default:
		panic(fmt.Sprintf("unknown resource type: %d", params.ResourceType))
	}

	_, err := s.queries.IdempotencyKeyComplete(ctx, arg)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}

	s.logger.DebugContext(ctx, "idempotency key completed",
		"idempotency_key", params.IdempotencyKey,
		"resource_type", params.ResourceType,
		"resource_id", params.ResourceID,
	)

	return nil
}

// Unreserve deletes a reservation row (only if response_bytes IS NULL).
// This allows the idempotency key to be retried after a handler error.
func (s *Store) Unreserve(ctx context.Context, key, userID uuid.UUID) error {
	_, err := s.queries.IdempotencyKeyUnreserve(ctx, db.IdempotencyKeyUnreserveParams{
		IdempotencyKey: key,
		UserID:         userID,
	})
	if err != nil {
		return fmt.Errorf("unreserve idempotency key: %w", err)
	}

	s.logger.DebugContext(ctx, "idempotency key unreserved",
		"idempotency_key", key,
	)

	return nil
}

// DeleteExpired removes expired idempotency keys.
func (s *Store) DeleteExpired(ctx context.Context) (int64, error) {
	rows, err := s.queries.IdempotencyKeyDeleteExpired(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired idempotency keys: %w", err)
	}
	return rows, nil
}

// StartCleanup runs a background goroutine that periodically deletes expired
// idempotency keys. It returns when ctx is cancelled.
func (s *Store) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := s.DeleteExpired(ctx)
			if err != nil {
				s.logger.Error("failed to cleanup expired idempotency keys", "error", err)
			} else if deleted > 0 {
				s.logger.Info("cleaned up expired idempotency keys", "count", deleted)
			}
		}
	}
}
