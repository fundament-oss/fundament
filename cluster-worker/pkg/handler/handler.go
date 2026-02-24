package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// EntityType identifies which outbox FK column a handler cares about.
type EntityType string

const (
	EntityCluster       EntityType = "cluster"
	EntityNamespace     EntityType = "namespace"
	EntityProjectMember EntityType = "project_member"
	EntityProject       EntityType = "project"
)

// SyncHandler processes an outbox row for a specific entity type.
// Implementations must be idempotent — the same ID may be delivered more than once
// (retries, reconciliation). Handlers must also gracefully handle cases where the
// entity referenced by the outbox row might no longer exist (e.g., deleted
// between outbox insertion and processing). Return nil to mark the row as processed.
type SyncHandler interface {
	Sync(ctx context.Context, id uuid.UUID) error
}

// StatusHandler checks the current state of synced objects and updates the DB.
// Called periodically by the status worker. Implementations should check whether
// reality matches desired state and update status columns accordingly.
type StatusHandler interface {
	CheckStatus(ctx context.Context) error
}

// ReconcileHandler performs cross-system reconciliation (e.g. orphan detection).
// Called periodically by the outbox worker's reconcile loop.
type ReconcileHandler interface {
	ReconcileOrphans(ctx context.Context) error
}

// Registry holds all registered handlers. The outbox worker and status worker
// use this to discover which handlers exist and route work to them.
type Registry struct {
	syncHandlers      map[EntityType]SyncHandler
	statusHandlers    []StatusHandler
	reconcileHandlers []ReconcileHandler
}

func NewRegistry() *Registry {
	return &Registry{
		syncHandlers: make(map[EntityType]SyncHandler),
	}
}

// RegisterSync registers a SyncHandler for an entity type.
// Panics if a handler is already registered for that type (catch wiring bugs early).
func (r *Registry) RegisterSync(entityType EntityType, h SyncHandler) {
	if _, exists := r.syncHandlers[entityType]; exists {
		panic(fmt.Sprintf("duplicate sync handler for %s", entityType))
	}
	r.syncHandlers[entityType] = h
}

// RegisterStatus registers a StatusHandler to be polled periodically.
func (r *Registry) RegisterStatus(h StatusHandler) {
	r.statusHandlers = append(r.statusHandlers, h)
}

// SyncHandlerFor returns the handler for an entity type, or an error if none is registered.
func (r *Registry) SyncHandlerFor(entityType EntityType) (SyncHandler, error) {
	h, ok := r.syncHandlers[entityType]
	if !ok {
		return nil, fmt.Errorf("no sync handler registered for %s", entityType)
	}
	return h, nil
}

// RegisterReconcile registers a ReconcileHandler to be called during reconciliation.
func (r *Registry) RegisterReconcile(h ReconcileHandler) {
	r.reconcileHandlers = append(r.reconcileHandlers, h)
}

// StatusHandlers returns all registered status handlers.
func (r *Registry) StatusHandlers() []StatusHandler {
	return r.statusHandlers
}

// ReconcileHandlers returns all registered reconcile handlers.
func (r *Registry) ReconcileHandlers() []ReconcileHandler {
	return r.reconcileHandlers
}
