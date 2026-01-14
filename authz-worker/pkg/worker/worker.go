package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/common/authz"

	openfga "github.com/openfga/go-sdk"
)

// Config holds configuration for the authz worker.
type Config struct {
	PollInterval time.Duration
	BatchSize    int32
}

// Worker processes the authz outbox table and writes/deletes OpenFGA tuples.
type Worker struct {
	pool   *pgxpool.Pool
	authz  *authz.Client
	logger *slog.Logger
	cfg    Config
}

// New creates a new authz outbox worker.
func New(pool *pgxpool.Pool, authzClient *authz.Client, logger *slog.Logger, cfg Config) *Worker {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	return &Worker{
		pool:   pool,
		authz:  authzClient,
		logger: logger,
		cfg:    cfg,
	}
}

type outboxRow struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       json.RawMessage
}

// Run starts the worker loop. It blocks until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting authz outbox worker",
		"poll_interval", w.cfg.PollInterval,
		"batch_size", w.cfg.BatchSize,
	)

	// Start listening for notifications
	listenConn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection for LISTEN: %w", err)
	}
	defer listenConn.Release()

	_, err = listenConn.Exec(ctx, "LISTEN authz_outbox")
	if err != nil {
		return fmt.Errorf("failed to LISTEN: %w", err)
	}

	// Process any existing unprocessed rows on startup
	w.processBatch(ctx)

	for {
		// Wait for notification or poll interval
		waitCtx, cancel := context.WithTimeout(ctx, w.cfg.PollInterval)
		_, err := listenConn.Conn().WaitForNotification(waitCtx)
		cancel()

		if ctx.Err() != nil {
			w.logger.Info("shutting down authz outbox worker")
			return nil
		}

		if err != nil && waitCtx.Err() == nil {
			w.logger.Warn("error waiting for notification, falling back to poll", "error", err)
		}

		w.processBatch(ctx)
	}
}

const (
	minBackoff = 1 * time.Second
	maxBackoff = 60 * time.Second
)

func (w *Worker) processBatch(ctx context.Context) {
	backoff := minBackoff

	for {
		processed, hadErrors, err := w.processOneBatch(ctx)
		if err != nil {
			w.logger.Error("failed to process outbox batch", "error", err, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		if processed == 0 {
			return
		}

		w.logger.Debug("processed outbox batch", "count", processed, "had_errors", hadErrors)

		if hadErrors {
			// Some items failed to process — back off to avoid hammering OpenFGA
			w.logger.Warn("batch had processing errors, backing off", "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff = min(backoff*2, maxBackoff)
		} else {
			// Successful batch — reset backoff
			backoff = minBackoff
		}
	}
}

func (w *Worker) processOneBatch(ctx context.Context) (processed int, hadErrors bool, err error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	// Select and lock unprocessed rows
	rows, err := tx.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload
		FROM authz.outbox
		WHERE processed_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, w.cfg.BatchSize)
	if err != nil {
		return 0, false, fmt.Errorf("failed to query outbox: %w", err)
	}

	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (outboxRow, error) {
		var r outboxRow
		if err := row.Scan(&r.ID, &r.AggregateType, &r.AggregateID, &r.EventType, &r.Payload); err != nil {
			return r, fmt.Errorf("failed to scan outbox row: %w", err)
		}
		return r, nil
	})
	if err != nil {
		return 0, false, fmt.Errorf("failed to collect outbox rows: %w", err)
	}

	if len(items) == 0 {
		return 0, false, nil
	}

	// Process each item — only mark as processed on success, leave failed items for retry
	for i := range items {
		processErr := w.processItem(ctx, &items[i])
		if processErr != nil {
			w.logger.Error("failed to process outbox item, will retry",
				"id", items[i].ID,
				"aggregate_type", items[i].AggregateType,
				"aggregate_id", items[i].AggregateID,
				"event_type", items[i].EventType,
				"error", processErr,
			)
			hadErrors = true
			continue
		}

		_, err := tx.Exec(ctx, `
			UPDATE authz.outbox
			SET processed_at = now()
			WHERE id = $1
		`, items[i].ID)
		if err != nil {
			return 0, false, fmt.Errorf("failed to mark outbox item as processed: %w", err)
		}
		processed++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return processed, hadErrors, nil
}

func (w *Worker) processItem(ctx context.Context, item *outboxRow) error {
	eventKey := item.AggregateType + "." + item.EventType

	switch eventKey {
	case "user.created":
		return w.handleUserCreated(ctx, item.Payload)
	case "user.deleted":
		return w.handleUserDeleted(ctx, item.Payload)
	case "user.role_changed":
		return w.handleUserRoleChanged(ctx, item.Payload)
	case "project.created":
		return w.handleProjectCreated(ctx, item.Payload)
	case "project.deleted":
		return w.handleProjectDeleted(ctx, item.Payload)
	case "project_member.created":
		return w.handleProjectMemberCreated(ctx, item.Payload)
	case "project_member.deleted":
		return w.handleProjectMemberDeleted(ctx, item.Payload)
	case "project_member.role_changed":
		return w.handleProjectMemberRoleChanged(ctx, item.Payload)
	default:
		w.logger.Warn("unknown event type, skipping", "event", eventKey)
		return nil
	}
}

// --- User events ---

type userPayload struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
}

type userRoleChangedPayload struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	OldRole        string `json:"old_role"`
	NewRole        string `json:"new_role"`
}

func (w *Worker) handleUserCreated(ctx context.Context, payload json.RawMessage) error {
	var p userPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal user.created payload: %w", err)
	}
	user := "user:" + p.UserID
	org := "organization:" + p.OrganizationID

	tuples := []openfga.TupleKey{authz.Tuple(user, "member", org)}
	if p.Role == "admin" {
		tuples = append(tuples, authz.Tuple(user, "admin", org))
	}
	if err := w.authz.WriteTuples(ctx, tuples...); err != nil {
		return fmt.Errorf("failed to write user.created tuples: %w", err)
	}
	return nil
}

func (w *Worker) handleUserDeleted(ctx context.Context, payload json.RawMessage) error {
	var p userPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal user.deleted payload: %w", err)
	}
	user := "user:" + p.UserID
	org := "organization:" + p.OrganizationID

	// Delete both member and admin — if admin didn't exist, OpenFGA ignores the delete
	if err := w.authz.DeleteTuples(ctx,
		authz.TupleDelete(user, "member", org),
		authz.TupleDelete(user, "admin", org),
	); err != nil {
		return fmt.Errorf("failed to delete user.deleted tuples: %w", err)
	}
	return nil
}

func (w *Worker) handleUserRoleChanged(ctx context.Context, payload json.RawMessage) error {
	var p userRoleChangedPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal user.role_changed payload: %w", err)
	}
	user := "user:" + p.UserID
	org := "organization:" + p.OrganizationID

	if err := w.authz.DeleteTuples(ctx, authz.TupleDelete(user, p.OldRole, org)); err != nil {
		return fmt.Errorf("failed to delete old role tuple: %w", err)
	}
	if err := w.authz.WriteTuples(ctx, authz.Tuple(user, p.NewRole, org)); err != nil {
		return fmt.Errorf("failed to write new role tuple: %w", err)
	}
	return nil
}

// --- Project events ---

type projectPayload struct {
	ProjectID      string `json:"project_id"`
	OrganizationID string `json:"organization_id"`
}

func (w *Worker) handleProjectCreated(ctx context.Context, payload json.RawMessage) error {
	var p projectPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal project.created payload: %w", err)
	}
	if err := w.authz.WriteTuples(ctx,
		authz.Tuple("organization:"+p.OrganizationID, "organization", "project:"+p.ProjectID),
	); err != nil {
		return fmt.Errorf("failed to write project.created tuple: %w", err)
	}
	return nil
}

func (w *Worker) handleProjectDeleted(ctx context.Context, payload json.RawMessage) error {
	var p projectPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal project.deleted payload: %w", err)
	}
	if err := w.authz.DeleteTuples(ctx,
		authz.TupleDelete("organization:"+p.OrganizationID, "organization", "project:"+p.ProjectID),
	); err != nil {
		return fmt.Errorf("failed to delete project.deleted tuple: %w", err)
	}
	return nil
}

// --- Project member events ---

type projectMemberPayload struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
}

type projectMemberRoleChangedPayload struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	OldRole   string `json:"old_role"`
	NewRole   string `json:"new_role"`
}

func (w *Worker) handleProjectMemberCreated(ctx context.Context, payload json.RawMessage) error {
	var p projectMemberPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal project_member.created payload: %w", err)
	}
	if err := w.authz.WriteTuples(ctx,
		authz.Tuple("user:"+p.UserID, p.Role, "project:"+p.ProjectID),
	); err != nil {
		return fmt.Errorf("failed to write project_member.created tuple: %w", err)
	}
	return nil
}

func (w *Worker) handleProjectMemberDeleted(ctx context.Context, payload json.RawMessage) error {
	var p projectMemberPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal project_member.deleted payload: %w", err)
	}
	if err := w.authz.DeleteTuples(ctx,
		authz.TupleDelete("user:"+p.UserID, p.Role, "project:"+p.ProjectID),
	); err != nil {
		return fmt.Errorf("failed to delete project_member.deleted tuple: %w", err)
	}
	return nil
}

func (w *Worker) handleProjectMemberRoleChanged(ctx context.Context, payload json.RawMessage) error {
	var p projectMemberRoleChangedPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal project_member.role_changed payload: %w", err)
	}
	user := "user:" + p.UserID
	project := "project:" + p.ProjectID

	if err := w.authz.DeleteTuples(ctx, authz.TupleDelete(user, p.OldRole, project)); err != nil {
		return fmt.Errorf("failed to delete old project member role tuple: %w", err)
	}
	if err := w.authz.WriteTuples(ctx, authz.Tuple(user, p.NewRole, project)); err != nil {
		return fmt.Errorf("failed to write new project member role tuple: %w", err)
	}
	return nil
}
