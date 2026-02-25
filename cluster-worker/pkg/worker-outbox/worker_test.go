package worker_outbox

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

// mockDBTX implements db.DBTX for testing handleRowError.
type mockDBTX struct {
	execCalled     bool
	queryRowCalled bool
	execErr        error
	queryRowResult pgx.Row
}

func (m *mockDBTX) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	m.execCalled = true
	return pgconn.CommandTag{}, m.execErr
}

func (m *mockDBTX) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	panic("Query should not be called")
}

func (m *mockDBTX) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	m.queryRowCalled = true
	return m.queryRowResult
}

// mockRow implements pgx.Row for testing OutboxMarkRetry results.
type mockRow struct {
	retries int32
	err     error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if len(dest) > 0 {
		if p, ok := dest[0].(*int32); ok {
			*p = m.retries
		}
	}
	return nil
}

func TestEntityFromRow(t *testing.T) {
	id := uuid.New()
	row := &db.OutboxGetAndLockRow{
		ID:         uuid.New(),
		SubjectID:  id,
		EntityType: string(handler.EntityCluster),
	}

	entityType, entityID := entityFromRow(row)

	if entityType != handler.EntityCluster {
		t.Errorf("expected EntityCluster, got %q", entityType)
	}
	if entityID != id {
		t.Errorf("expected %s, got %s", id, entityID)
	}
}

func newTestWorker() *OutboxWorker {
	return &OutboxWorker{
		logger: slog.Default(),
		cfg: Config{
			MaxRetries:  10,
			BaseBackoff: 500 * time.Millisecond,
			MaxBackoff:  time.Minute,
		},
	}
}

func TestHandleProcessingError_RetryBelowMax(t *testing.T) {
	mock := &mockDBTX{queryRowResult: &mockRow{retries: 6}}
	w := newTestWorker()
	qtx := db.New(mock)
	row := &db.OutboxGetAndLockRow{ID: uuid.New(), Retries: 5}

	err := w.handleRowError(context.Background(), qtx, row, errors.New("sync failed"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.queryRowCalled {
		t.Error("expected OutboxMarkRetry (QueryRow) to be called")
	}
	if mock.execCalled {
		t.Error("did not expect OutboxMarkFailed (Exec) to be called")
	}
}

func TestHandleProcessingError_ExceedMaxRetries(t *testing.T) {
	mock := &mockDBTX{}
	w := newTestWorker()
	qtx := db.New(mock)
	row := &db.OutboxGetAndLockRow{ID: uuid.New(), Retries: 9}

	err := w.handleRowError(context.Background(), qtx, row, errors.New("sync failed"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.execCalled {
		t.Error("expected OutboxMarkFailed (Exec) to be called")
	}
	if mock.queryRowCalled {
		t.Error("did not expect OutboxMarkRetry (QueryRow) to be called")
	}
}

func TestHandleProcessingError_MarkRetryFails(t *testing.T) {
	mock := &mockDBTX{queryRowResult: &mockRow{err: errors.New("db down")}}
	w := newTestWorker()
	qtx := db.New(mock)
	row := &db.OutboxGetAndLockRow{ID: uuid.New(), Retries: 5}

	err := w.handleRowError(context.Background(), qtx, row, errors.New("sync failed"))

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mark outbox retry") {
		t.Errorf("expected error to contain 'mark outbox retry', got: %v", err)
	}
}

func TestHandleProcessingError_MarkFailedFails(t *testing.T) {
	mock := &mockDBTX{execErr: errors.New("db down")}
	w := newTestWorker()
	qtx := db.New(mock)
	row := &db.OutboxGetAndLockRow{ID: uuid.New(), Retries: 9}

	err := w.handleRowError(context.Background(), qtx, row, errors.New("sync failed"))

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mark outbox failed") {
		t.Errorf("expected error to contain 'mark outbox failed', got: %v", err)
	}
}
