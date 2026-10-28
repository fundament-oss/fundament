// Package gardener provides interfaces and implementations for interacting with Gardener.
// It supports two client modes:
// - MockClient: In-memory, for unit/integration tests
// - RealClient: Actual Gardener API, for production and local Gardener
package gardener

import (
	"context"
	"reflect"
	"time"

	"github.com/google/uuid"

	commongardener "github.com/fundament-oss/fundament/common/gardener"
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
	// StatusLost is fundament's own: the Shoot of an active cluster
	// disappeared after fundament had seen it. Gardener never reports it.
	StatusLost ShootStatusType = "lost"
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
	// APIServerAvailable is Gardener's APIServerAvailable condition: its care
	// controller reached the shoot's /healthz. During a create this turns True
	// well before the Shoot is ready, while the worker nodes still join.
	APIServerAvailable bool
	// EveryNodeReadyMessage is the EveryNodeReady condition's message while the
	// condition is not satisfied; it names the machine deployments that wait.
	EveryNodeReadyMessage string
	// LastErrors are the Shoot's .status.lastErrors descriptions.
	LastErrors []string
}

// equal compares every field. ShootStatus is not comparable as a value
// (LastErrors is a slice), and DeepEqual keeps the comparison exhaustive for
// fields added later — the watch's change filter depends on that.
func (s *ShootStatus) equal(other *ShootStatus) bool {
	return reflect.DeepEqual(s, other)
}

// Status message constants for consistent messaging.
const (
	MsgShootNotFound = "Shoot not found in Gardener"
	// MsgShootLost is the message of a lost cluster.
	MsgShootLost      = "The cluster no longer exists in Gardener; changing the cluster creates it again, empty"
	MsgShootReady     = "Shoot is ready"
	MsgShootUnhealthy = "Shoot reconciled but not all conditions healthy"
	// MsgShootAwaitingHealth is shown from the end of a create until Gardener
	// first reports the new cluster healthy. Shared: organization-api keys the
	// STARTING cluster status on this exact text.
	MsgShootAwaitingHealth = commongardener.MsgShootAwaitingHealth
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

	// DeleteShootByClusterID deletes a Shoot by cluster ID label, and reports
	// whether there was one.
	DeleteShootByClusterID(ctx context.Context, clusterID uuid.UUID) (found bool, err error)

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
}

// ShootInfo contains information about a Shoot retrieved from Gardener.
type ShootInfo struct {
	Name      string
	Namespace string
	ClusterID uuid.UUID
	Labels    map[string]string
	// Deleting is set once the Shoot's deletion has started.
	Deleting bool
	// UID, KubernetesVersion, Region and Created feed the Gardener inventory
	// that drift detection compares against the database (FUN-23).
	UID               string
	KubernetesVersion string
	Region            string
	Created           time.Time
}
