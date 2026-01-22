// Package gardener provides interfaces and implementations for interacting with Gardener.
// It supports two client modes:
// - MockClient: In-memory, for unit/integration tests
// - RealClient: Actual Gardener API, for production and local Gardener
package gardener

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Shoot status constants returned by GetShootStatus.
const (
	StatusPending     = "pending"
	StatusProgressing = "progressing"
	StatusReady       = "ready"
	StatusError       = "error"
	StatusDeleting    = "deleting"
	StatusDeleted     = "deleted"
)

// Status message constants for consistent messaging.
const (
	MsgShootNotFound = "Shoot not found in Gardener"
	MsgShootReady    = "Shoot is ready"
)

// Client is the interface that all Gardener client implementations must satisfy.
// This allows swapping between mock (tests) and real (production/local Gardener).
type Client interface {
	// EnsureProject creates the Gardener Project if it doesn't exist (idempotent).
	// Project names are deterministic: sanitize(orgName)[:6] + hash(orgName)[:4]
	// Returns the actual namespace created by Gardener (read from project.Status.Namespace).
	EnsureProject(ctx context.Context, projectName string, orgID uuid.UUID) (namespace string, err error)

	// ApplyShoot creates or updates a Shoot in Gardener.
	// Uses cluster ID label to find existing shoots (ignores ShootName for updates).
	// ShootName is only used when creating a new Shoot.
	ApplyShoot(ctx context.Context, cluster *ClusterToSync) error

	// DeleteShoot deletes a Shoot by cluster info (uses label-based lookup)
	DeleteShoot(ctx context.Context, cluster *ClusterToSync) error

	// DeleteShootByName deletes a Shoot by name (for orphan cleanup)
	DeleteShootByName(ctx context.Context, name string) error

	// GetShootByClusterID finds a Shoot by its cluster ID label.
	// Returns nil if not found.
	GetShootByClusterID(ctx context.Context, namespace string, clusterID uuid.UUID) (*ShootInfo, error)

	// ListShoots returns all Shoots managed by this worker
	ListShoots(ctx context.Context) ([]ShootInfo, error)

	// GetShootStatus returns the current reconciliation status of a Shoot.
	// Returns status ("pending", "progressing", "ready", "error", "deleting", "deleted")
	// and a descriptive message.
	GetShootStatus(ctx context.Context, cluster *ClusterToSync) (status string, message string, err error)
}

// ClusterToSync contains all the information needed to sync a cluster to Gardener.
type ClusterToSync struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID  // Organization UUID (for labels)
	OrganizationName  string     // Organization name (for reference, used in logging)
	Name              string     // Cluster name
	ShootName         string     // Generated Gardener Shoot name (used only on create)
	Namespace         string     // Gardener namespace (garden-{project-name})
	Region            string
	KubernetesVersion string
	Deleted           *time.Time
	SyncAttempts      int
}

// ShootInfo contains information about a Shoot retrieved from Gardener.
type ShootInfo struct {
	Name      string
	ClusterID uuid.UUID
	Labels    map[string]string
}
