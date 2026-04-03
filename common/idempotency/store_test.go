package idempotency

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func createTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	name := testNameToDbName(t.Name())

	adminPool, err := pgxpool.New(t.Context(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable",
		testDBPort,
	))
	if err != nil {
		t.Fatalf("failed to connect to admin: %v", err)
	}
	defer adminPool.Close()

	_, err = adminPool.Exec(t.Context(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	if err != nil {
		t.Fatalf("failed to drop test database: %v", err)
	}

	_, err = adminPool.Exec(t.Context(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Connect as superuser to bypass RLS.
	pool, err := pgxpool.New(t.Context(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable",
		testDBPort, name,
	))
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	pool := createTestDB(t)
	logger := slog.Default()
	return NewStore(pool, Config{}, logger)
}

func TestStore_ReserveThenLookupReturnsNilResponse(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	key := uuid.New()
	userID := uuid.New()

	reserved, err := store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         userID,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if !reserved {
		t.Fatal("expected reservation to succeed")
	}

	cached, err := store.Lookup(ctx, key, userID)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if cached == nil {
		t.Fatal("expected cached entry after reservation")
	}
	if cached.ResponseBytes != nil {
		t.Error("expected nil ResponseBytes for reservation")
	}
	if cached.Procedure != "/test.Service/Create" {
		t.Errorf("expected procedure '/test.Service/Create', got %q", cached.Procedure)
	}
}

func TestStore_DuplicateReserveReturnsFalse(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	key := uuid.New()
	userID := uuid.New()

	reserved, err := store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         userID,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if !reserved {
		t.Fatal("expected first reservation to succeed")
	}

	// Second reserve with same key+user should conflict.
	reserved, err = store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         userID,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if reserved {
		t.Fatal("expected second reservation to conflict (return false)")
	}
}

func TestStore_DifferentUsersCanUseSameKey(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	key := uuid.New()
	user1 := uuid.New()
	user2 := uuid.New()

	reserved1, err := store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         user1,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("reserve user1: %v", err)
	}
	if !reserved1 {
		t.Fatal("expected user1 reservation to succeed")
	}

	reserved2, err := store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         user2,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("reserve user2: %v", err)
	}
	if !reserved2 {
		t.Fatal("expected user2 reservation to succeed with same key")
	}
}

func TestStore_CompleteUpdatesReservation(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	key := uuid.New()
	userID := uuid.New()
	// Use existing project from test data to satisfy FK constraint.
	resourceID := uuid.MustParse("019b4000-9000-7000-8000-000000000001")

	_, err := store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         userID,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	err = store.Complete(ctx, &CompleteParams{
		IdempotencyKey: key,
		UserID:         userID,
		ResponseBytes:  []byte("response-data"),
		ResourceType:   ResourceProject,
		ResourceID:     resourceID,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	cached, err := store.Lookup(ctx, key, userID)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if cached == nil {
		t.Fatal("expected cached entry after completion")
	}
	if string(cached.ResponseBytes) != "response-data" {
		t.Errorf("expected response 'response-data', got %q", cached.ResponseBytes)
	}
	if cached.ResourceID != resourceID {
		t.Errorf("expected resource ID %v, got %v", resourceID, cached.ResourceID)
	}
}

func TestStore_UnreserveAllowsRetry(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	key := uuid.New()
	userID := uuid.New()

	// Reserve.
	_, err := store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         userID,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	// Unreserve (simulates handler error).
	err = store.Unreserve(ctx, key, userID)
	if err != nil {
		t.Fatalf("unreserve: %v", err)
	}

	// Lookup should return nil (deleted).
	cached, err := store.Lookup(ctx, key, userID)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if cached != nil {
		t.Fatal("expected nil after unreserve")
	}

	// Reserve again should succeed.
	reserved, err := store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         userID,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if !reserved {
		t.Fatal("expected re-reservation to succeed after unreserve")
	}
}

func TestStore_UnreserveDoesNotDeleteCompletedEntry(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	key := uuid.New()
	userID := uuid.New()
	// Use existing project from test data to satisfy FK constraint.
	resourceID := uuid.MustParse("019b4000-9000-7000-8000-000000000001")

	// Reserve + Complete.
	_, err := store.Reserve(ctx, ReserveParams{
		IdempotencyKey: key,
		UserID:         userID,
		Procedure:      "/test.Service/Create",
		RequestHash:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	err = store.Complete(ctx, &CompleteParams{
		IdempotencyKey: key,
		UserID:         userID,
		ResponseBytes:  []byte("response-data"),
		ResourceType:   ResourceProject,
		ResourceID:     resourceID,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Unreserve should NOT delete the completed entry (response_bytes IS NOT NULL).
	err = store.Unreserve(ctx, key, userID)
	if err != nil {
		t.Fatalf("unreserve: %v", err)
	}

	// Lookup should still return the completed entry.
	cached, err := store.Lookup(ctx, key, userID)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if cached == nil {
		t.Fatal("expected completed entry to survive unreserve")
	}
	if string(cached.ResponseBytes) != "response-data" {
		t.Errorf("expected response 'response-data', got %q", cached.ResponseBytes)
	}
}
