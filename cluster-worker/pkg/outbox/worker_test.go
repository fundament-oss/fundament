package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

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
		ID:        uuid.New(),
		ClusterID: pgtype.UUID{Bytes: id, Valid: true},
	}

	entityType, entityID, err := entityFromRow(row)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entityType != handler.EntityCluster {
		t.Errorf("expected EntityCluster, got %q", entityType)
	}
	if entityID != id {
		t.Errorf("expected %s, got %s", id, entityID)
	}
}

func TestEntityFromRow_NoValidFK(t *testing.T) {
	row := &db.OutboxGetAndLockRow{
		ID: uuid.New(),
	}

	_, _, err := entityFromRow(row)

	if err == nil {
		t.Fatal("expected error for row with no valid FK")
	}
}

func TestProcessWrapsInvalidEntityAsNonRetryable(t *testing.T) {
	w := newTestWorker()
	row := &db.OutboxGetAndLockRow{ID: uuid.New()} // no valid FK

	_, err := w.process(context.Background(), row)

	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errNonRetryable) {
		t.Errorf("expected errNonRetryable, got: %v", err)
	}
}

func TestEntityFromRow_NodePoolID(t *testing.T) {
	id := uuid.New()
	row := &db.OutboxGetAndLockRow{
		ID:         uuid.New(),
		NodePoolID: pgtype.UUID{Bytes: id, Valid: true},
	}

	entityType, entityID, err := entityFromRow(row)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entityType != handler.EntityNodePool {
		t.Errorf("expected EntityNodePool, got %q", entityType)
	}
	if entityID != id {
		t.Errorf("expected %s, got %s", id, entityID)
	}
}

func TestEntityFromRow_NamespaceID(t *testing.T) {
	id := uuid.New()
	row := &db.OutboxGetAndLockRow{
		ID:          uuid.New(),
		NamespaceID: pgtype.UUID{Bytes: id, Valid: true},
	}

	entityType, entityID, err := entityFromRow(row)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entityType != handler.EntityNamespace {
		t.Errorf("expected EntityNamespace, got %q", entityType)
	}
	if entityID != id {
		t.Errorf("expected %s, got %s", id, entityID)
	}
}

func newTestWorker() *Worker {
	return &Worker{
		logger: slog.Default(),
		cfg: Config{
			MaxRetries:               10,
			BaseBackoff:              500 * time.Millisecond,
			MaxBackoff:               time.Minute,
			PreconditionDelay:        30 * time.Second,
			MaxPreconditionDeferrals: 100,
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

// A row fails at once on an error a retry cannot fix, and is retried on one
// that can pass: an outage, a conflict, or forbidden while something settles.
// Gardener's admission refusing a Shoot spec is forbidden on the Shoot.
func TestFailsNow(t *testing.T) {
	shoot := schema.GroupResource{Group: "core.gardener.cloud", Resource: "shoots"}
	namespaces := schema.GroupResource{Resource: "namespaces"}
	invalid := apierrors.NewInvalid(schema.GroupKind{Group: "core.gardener.cloud", Kind: "Shoot"}, "c1",
		field.ErrorList{field.NotSupported(field.NewPath("spec", "kubernetes", "version"), "1.31.0", []string{"1.33.0"})})

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "non-retryable", err: fmt.Errorf("process: %w", errNonRetryable), want: true},
		{name: "invalid, wrapped by the handler", err: fmt.Errorf("apply shoot: %w", invalid), want: true},
		{name: "invalid, joined with another error", err: errors.Join(errors.New("other"), invalid), want: true},
		{name: "bad request", err: apierrors.NewBadRequest("malformed"), want: true},
		{name: "shoot spec refused by Gardener's admission", err: fmt.Errorf("apply shoot: %w", apierrors.NewForbidden(shoot, "c1", errors.New(`spec.kubernetes.version: Unsupported value: "1.31.0"`))), want: true},
		{name: "the worker not yet allowed on the project's shoots", err: fmt.Errorf("apply shoot: %w", apierrors.NewForbidden(shoot, "c1", errors.New(`User "system:serviceaccount:garden-acme:fundament" cannot create resource "shoots" in API group "core.gardener.cloud" in the namespace "garden-acme"`))), want: false},
		{name: "forbidden on a terminating namespace", err: apierrors.NewForbidden(namespaces, "tnt-a", errors.New("namespace is being terminated")), want: false},
		{name: "conflict", err: apierrors.NewConflict(shoot, "c1", errors.New("modified")), want: false},
		{name: "unavailable", err: apierrors.NewServiceUnavailable("down"), want: false},
		{name: "timeout", err: apierrors.NewTimeoutError("slow", 5), want: false},
		{name: "plain error", err: errors.New("connection refused"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, failsNow(tt.err))
		})
	}
}
