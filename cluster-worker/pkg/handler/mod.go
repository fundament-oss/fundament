package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/dbconst"
)

// PreconditionError signals that a structural precondition is not met.
// The outbox worker defers the row without incrementing retries.
type PreconditionError struct {
	Reason string
}

func (e *PreconditionError) Error() string {
	return "precondition not met: " + e.Reason
}

func NewPreconditionError(reason string) *PreconditionError {
	return &PreconditionError{Reason: reason}
}

// EntityType identifies an entity type in the outbox table via its FK column.
type EntityType string

const (
	EntityCluster       EntityType = "cluster"
	EntityOrgUser       EntityType = "org_user"
	EntityProjectMember EntityType = "project_member"
	EntityNodePool      EntityType = "node_pool"
	EntityNamespace     EntityType = "namespace"
)

// SyncContext carries metadata from the outbox row to the sync handler.
type SyncContext struct {
	EntityType EntityType                  // which entity type this row represents
	Event      dbconst.ClusterOutboxEvent  // created, updated, deleted, reconcile, ready
	Source     dbconst.ClusterOutboxSource // trigger, reconcile, manual, node_pool, status
}

// SyncHandler processes an outbox row for a specific entity type.
// Implementations must be idempotent — the same ID may be delivered more than once
// (retries, reconciliation). Handlers must also gracefully handle cases where the
// entity referenced by the outbox row might no longer exist (e.g., deleted
// between outbox insertion and processing). Return nil to mark the row as processed.
type SyncHandler interface {
	Sync(ctx context.Context, id uuid.UUID, sc SyncContext) error
}

// StatusHandler checks the current state of synced objects and updates the DB.
// Called periodically by the status worker. Implementations should check whether
// reality matches desired state and update status columns accordingly.
type StatusHandler interface {
	CheckStatus(ctx context.Context) error
}

// ReconcileHandler performs periodic reconciliation for an entity type.
// Implementations should:
//   - Re-enqueue outbox rows for entities with unsynced changes (feeding the SyncHandler)
//   - Detect and clean up orphaned resources in external systems
//
// Called periodically by the outbox worker's reconcile loop.
type ReconcileHandler interface {
	Reconcile(ctx context.Context) error
}

// Precondition declares what must be true before a handler can process an entity.
type Precondition struct {
	Description string
	Check       func(ctx context.Context, id uuid.UUID) error // returns PreconditionError or nil
}

// RouteKey identifies a handler registration by entity type and optional event.
// A zero-value Event (empty string) represents the default handler for that entity type.
type RouteKey struct {
	Entity EntityType
	Event  dbconst.ClusterOutboxEvent
}

// Registry holds all registered handlers. The outbox worker and status worker
// use this to discover which handlers exist and route work to them.
type Registry struct {
	// syncHandlers holds the default (non-event-specific) handler per entity type.
	syncHandlers map[EntityType]SyncHandler
	// eventSyncHandlers holds event-specific handlers. An event may have more
	// than one handler — e.g. both usersync and namespace-sync react to a
	// cluster becoming ready — so they are dispatched as a fan-out.
	eventSyncHandlers map[RouteKey][]SyncHandler
	statusHandlers    []StatusHandler
	reconcileHandlers []ReconcileHandler
}

func NewRegistry() *Registry {
	return &Registry{
		syncHandlers:      make(map[EntityType]SyncHandler),
		eventSyncHandlers: make(map[RouteKey][]SyncHandler),
	}
}

// RegisterSync registers the default SyncHandler for an entity type (non-event-specific).
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

// RegisterReconcile registers a ReconcileHandler to be called during reconciliation.
func (r *Registry) RegisterReconcile(h ReconcileHandler) {
	r.reconcileHandlers = append(r.reconcileHandlers, h)
}

// RegisterSyncForEvent registers a SyncHandler for a specific entity type + event combination.
// Event-specific handlers take precedence over the default entity-type handler.
// Multiple handlers may register for the same combination; all of them are
// dispatched (fan-out) when a matching row is processed — e.g. both usersync
// and namespace-sync react to a cluster's ready event.
func (r *Registry) RegisterSyncForEvent(entityType EntityType, event dbconst.ClusterOutboxEvent, h SyncHandler) {
	key := RouteKey{Entity: entityType, Event: event}
	r.eventSyncHandlers[key] = append(r.eventSyncHandlers[key], h)
}

// SyncHandlersFor returns the handlers for an entity type and event.
// Event-specific handlers take precedence; if none are registered for the event,
// it falls back to the default entity-type handler. Returns an error if neither exists.
func (r *Registry) SyncHandlersFor(entityType EntityType, event dbconst.ClusterOutboxEvent) ([]SyncHandler, error) {
	if hs, ok := r.eventSyncHandlers[RouteKey{Entity: entityType, Event: event}]; ok && len(hs) > 0 {
		return hs, nil
	}
	if h, ok := r.syncHandlers[entityType]; ok {
		return []SyncHandler{h}, nil
	}
	return nil, fmt.Errorf("no sync handler registered for %s (event=%s)", entityType, event)
}

// StatusHandlers returns all registered status handlers.
func (r *Registry) StatusHandlers() []StatusHandler {
	return r.statusHandlers
}

// ReconcileHandlers returns all registered reconcile handlers.
func (r *Registry) ReconcileHandlers() []ReconcileHandler {
	return r.reconcileHandlers
}
