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

// OperationType is the Gardener operation a status was derived from
// (the Shoot's .status.lastOperation.type). Values mirror Gardener's.
type OperationType string

const (
	OperationCreate    OperationType = "Create"
	OperationReconcile OperationType = "Reconcile"
	OperationDelete    OperationType = "Delete"
	OperationMigrate   OperationType = "Migrate"
	OperationRestore   OperationType = "Restore"
)

// ShootStatus contains the current status and a descriptive message.
type ShootStatus struct {
	Status  ShootStatusType
	Message string
	// Operation is empty when the Shoot has no last operation yet.
	Operation OperationType
	// Healthy is Gardener's health verdict: its shoot.gardener.cloud/status
	// label is "healthy", or, for a Shoot without the label, all required
	// conditions are True. Only meaningful for StatusReady and for a
	// StatusProgressing reconcile of a shoot that is already running.
	Healthy bool
	// HealthGrace marks the label value "progressing": a condition turned bad
	// but is within Gardener's grace period (its threshold, or pardoned while
	// an operation runs without errors). Healthy is false.
	HealthGrace bool
	// Retrying marks a StatusError that Gardener will retry by itself
	// (lastOperation state Error or Aborted), as opposed to Failed.
	Retrying bool
}

// Status message constants for consistent messaging.
const (
	MsgShootNotFound  = "Shoot not found in Gardener"
	MsgShootReady     = "Shoot is ready"
	MsgShootUnhealthy = "Shoot reconciled but not all conditions healthy"
	// MsgShootAwaitingHealth is shown from the end of a create until Gardener
	// first reports the new cluster healthy.
	MsgShootAwaitingHealth = "Cluster is ready, waiting for health checks to pass"
	// MsgShootUpdatePending is shown from the moment Gardener accepts a spec
	// change until the gardenlet starts reconciling it.
	MsgShootUpdatePending = "Waiting for Gardener to start the update"
)

// AdminKubeconfig holds the result of an AdminKubeconfigRequest.
type AdminKubeconfig struct {
	Kubeconfig []byte
	ExpiresAt  time.Time
}

// Client is the interface that all Gardener client implementations must satisfy.
// This allows swapping between mock (tests) and real (production/local Gardener).
type Client interface {
	// EnsureProject creates the Gardener Project if it doesn't exist (idempotent).
	// Project names are deterministic: sanitize(orgName)[:6] + hash(orgName)[:4]
	// Returns the actual namespace created by Gardener (read from project.Status.Namespace).
	EnsureProject(ctx context.Context, projectName string, orgID uuid.UUID) (namespace string, err error)

	// ApplyShoot creates or updates a Shoot in Gardener.
	// Uses cluster ID label to find existing shoots. specChanged reports that an
	// existing Shoot's spec changed, so Gardener will reconcile it.
	ApplyShoot(ctx context.Context, cluster *ClusterToSync) (specChanged bool, err error)

	// DeleteShootByClusterID deletes a Shoot by cluster ID label.
	DeleteShootByClusterID(ctx context.Context, clusterID uuid.UUID) error

	// ListShoots returns all Shoots managed by this worker
	ListShoots(ctx context.Context) ([]ShootInfo, error)

	// GetShootStatus returns the current reconciliation status of a Shoot.
	GetShootStatus(ctx context.Context, cluster *ClusterToSync) (*ShootStatus, error)

	// RequestAdminKubeconfig requests a short-lived admin kubeconfig for a shoot.
	// The kubeconfig provides cluster-admin access to the shoot's kube-apiserver.
	RequestAdminKubeconfig(ctx context.Context, clusterID uuid.UUID, expirationSeconds int64) (*AdminKubeconfig, error)
}

// NodePool represents a node pool configuration from the database.
type NodePool struct {
	Name         string
	MachineType  string
	AutoscaleMin int32
	AutoscaleMax int32
	// Zone is the availability zone for this worker pool (e.g., "eu-central-1a").
	// Empty string means no zone constraint, which is required for the local provider.
	// TODO: populate from tenant.node_pools once a zone column is added to the schema.
	Zone string
}

// ClusterToSync contains all the information needed to sync a cluster to Gardener.
type ClusterToSync struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID // Organization UUID (for labels)
	OrganizationName  string    // Organization name (for Gardener project naming)
	Name              string    // Cluster name
	ShootName         string    // Generated Gardener Shoot name (used only on create)
	Namespace         string    // Gardener namespace (garden-{project-name})
	Region            string
	KubernetesVersion string
	// CloudProfile / CloudProfileRegion select the gardener CloudProfile and the
	// region within it; empty falls back to the provider defaults.
	CloudProfile       string
	CloudProfileRegion string
	Deleted            *time.Time
	NodePools          []NodePool // Node pool configurations for Gardener worker groups
	NodeLimits         NodeLimits // Owning organization's node caps, applied at Shoot build time
}

// NodeLimits are an organization's node caps from tenant.organization_limits.
// A nil cap is unlimited. MaxNodesPerNodePool clamps each worker's autoscaler
// maximum; the other two have no Gardener field and fail the apply when
// exceeded (see clampWorkerMaxima / validateNodeLimits).
type NodeLimits struct {
	MaxNodesPerCluster     *int32
	MaxNodePoolsPerCluster *int32
	MaxNodesPerNodePool    *int32
}

// ShootInfo contains information about a Shoot retrieved from Gardener.
type ShootInfo struct {
	Name      string
	Namespace string
	ClusterID uuid.UUID
	Labels    map[string]string
}
