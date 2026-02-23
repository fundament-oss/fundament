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

// ShootStatusType represents the reconciliation state of a Shoot.
type ShootStatusType string

const (
	StatusPending     ShootStatusType = "pending"
	StatusProgressing ShootStatusType = "progressing"
	StatusReady       ShootStatusType = "ready"
	StatusError       ShootStatusType = "error"
	StatusDeleting    ShootStatusType = "deleting"
	StatusDeleted     ShootStatusType = "deleted"
)

// ShootStatus contains the current status and a descriptive message.
type ShootStatus struct {
	Status  ShootStatusType
	Message string
}

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
	// Uses cluster ID label to find existing shoots.
	ApplyShoot(ctx context.Context, cluster *ClusterToSync) error

	// DeleteShootByClusterID deletes a Shoot by cluster ID label.
	DeleteShootByClusterID(ctx context.Context, clusterID uuid.UUID) error

	// ListShoots returns all Shoots managed by this worker
	ListShoots(ctx context.Context) ([]ShootInfo, error)

	// GetShootStatus returns the current reconciliation status of a Shoot.
	GetShootStatus(ctx context.Context, cluster *ClusterToSync) (*ShootStatus, error)
}

// NodePool represents a node pool configuration from the database.
type NodePool struct {
	Name         string
	MachineType  string
	AutoscaleMin int32
	AutoscaleMax int32
}

// ClusterToSync contains all the information needed to sync a cluster to Gardener.
type ClusterToSync struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID // Organization UUID (for labels)
	OrganizationName  string    // Organization name (for reference, used in logging)
	Name              string    // Cluster name
	ShootName         string    // Generated Gardener Shoot name (used only on create)
	Namespace         string    // Gardener namespace (garden-{project-name})
	Region            string
	KubernetesVersion string
	Deleted           *time.Time
	SyncAttempts      int
	NodePools         []NodePool // Node pool configurations for Gardener worker groups
}

// ShootInfo contains information about a Shoot retrieved from Gardener.
type ShootInfo struct {
	Name      string
	Namespace string
	ClusterID uuid.UUID
	Labels    map[string]string
}
