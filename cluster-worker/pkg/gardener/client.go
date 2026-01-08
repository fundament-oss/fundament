// Package gardener provides interfaces and implementations for interacting with Gardener.
// It supports three client modes:
// - MockClient: In-memory, for unit/integration tests (Phase 1)
// - LocalClient: ConfigMaps in k3d, for local development (Phase 2)
// - RealClient: Actual Gardener API, for production (Phase 3)
package gardener

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Client is the interface that all Gardener client implementations must satisfy.
// This allows swapping between mock (tests), local (k3d development), and real (production).
type Client interface {
	// ApplyShoot creates or updates a Shoot in Gardener
	ApplyShoot(ctx context.Context, cluster ClusterToSync) error

	// DeleteShoot deletes a Shoot by cluster info
	DeleteShoot(ctx context.Context, cluster ClusterToSync) error

	// DeleteShootByName deletes a Shoot by name (for orphan cleanup)
	DeleteShootByName(ctx context.Context, name string) error

	// ListShoots returns all Shoots managed by this worker
	ListShoots(ctx context.Context) ([]ShootInfo, error)

	// GetShootStatus returns the current reconciliation status of a Shoot.
	// Returns status ("pending", "progressing", "ready", "error", "deleting", "deleted")
	// and a descriptive message.
	GetShootStatus(ctx context.Context, cluster ClusterToSync) (status string, message string, err error)
}

// ClusterToSync contains all the information needed to sync a cluster to Gardener.
type ClusterToSync struct {
	ID               uuid.UUID
	Name             string
	OrganizationName string
	Deleted          *time.Time
	SyncAttempts     int
}

// ShootInfo contains information about a Shoot retrieved from Gardener.
type ShootInfo struct {
	Name      string
	ClusterID uuid.UUID
	Labels    map[string]string
}

// ShootName returns the Gardener Shoot name for a cluster.
// Format: {organization}-{cluster}
func ShootName(organizationName, clusterName string) string {
	return organizationName + "-" + clusterName
}

// Hardcoded defaults (AWS-style values for MVP)
// These are example values using AWS-style naming. Replace with actual
// cloud provider region/zone when deploying.
const (
	DefaultRegion              = "nl-central-1" // Utrecht
	DefaultKubernetesVersion   = "1.29.4"
	DefaultMachineType         = "m5.xlarge"   // 4 vCPU, 16 GB RAM
	DefaultMachineImageName    = "gardenlinux" // Gardener's default Linux
	DefaultMachineImageVersion = "1592.1.0"
	DefaultVolumeType          = "gp3" // SSD
	DefaultVolumeSize          = "50Gi"
	DefaultZone                = "nl-central-1a"
)
