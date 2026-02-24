package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type stubHandler struct {
	lastID uuid.UUID
}

func (s *stubHandler) Sync(ctx context.Context, id uuid.UUID) error {
	s.lastID = id
	return nil
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	stub := &stubHandler{}

	r.RegisterSync(EntityCluster, stub)

	h, err := r.SyncHandlerFor(EntityCluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h != stub {
		t.Error("returned handler does not match registered handler")
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	r := NewRegistry()
	stub := &stubHandler{}

	r.RegisterSync(EntityCluster, stub)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()

	r.RegisterSync(EntityCluster, stub)
}

func TestRegistry_UnknownEntityReturnsError(t *testing.T) {
	r := NewRegistry()

	_, err := r.SyncHandlerFor(EntityCluster)
	if err == nil {
		t.Error("expected error for unregistered entity type")
	}
}
