// Package gardener provides interfaces and implementations for interacting with Gardener.
// It supports two client modes:
// - MockClient: In-memory, for unit/integration tests
// - RealClient: Actual Gardener API, for production and local Gardener
package gardener

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	// ApplyShoot creates or updates a Shoot in Gardener
	ApplyShoot(ctx context.Context, cluster *ClusterToSync) error

	// DeleteShoot deletes a Shoot by cluster info
	DeleteShoot(ctx context.Context, cluster *ClusterToSync) error

	// MaxShootNameLength returns the maximum allowed shoot name length for this client.
	// The local provider has a restrictive limit (21), while other providers can use up to 63.
	MaxShootNameLength() int

	// DeleteShootByName deletes a Shoot by name (for orphan cleanup)
	DeleteShootByName(ctx context.Context, name string) error

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
	Name              string
	OrganizationName  string
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

// hashSuffixLength is the length of the hash suffix used for long names.
const hashSuffixLength = 8

// ShootName returns the Gardener Shoot name for a cluster.
// Format: {organization}-{cluster}
// If the combined name exceeds maxLen, it returns a shortened version in the
// format: {prefix}-{hash} where the prefix is as much of the original name
// as fits within the limit.
func ShootName(organizationName, clusterName string, maxLen int) string {

	fullName := organizationName + "-" + clusterName

	if len(fullName) <= maxLen {
		return fullName
	}

	// Create a hash of the full name for uniqueness
	hash := sha256.Sum256([]byte(fullName))
	hashStr := hex.EncodeToString(hash[:])[:hashSuffixLength]

	// Calculate prefix length: maxLen - 1 (hyphen) - hashLen
	prefixLen := maxLen - 1 - hashSuffixLength

	// Take as much of the original name as fits
	prefix := fullName
	if len(prefix) > prefixLen {
		prefix = prefix[:prefixLen]
	}

	return prefix + "-" + hashStr
}
