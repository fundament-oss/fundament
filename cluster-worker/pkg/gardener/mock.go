package gardener

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MockClient implements Client for testing.
// It stores shoots in-memory and tracks all calls for test assertions.
// Features:
//   - Status progression: shoots progress through pending → progressing → ready
//   - Spec validation: validates cluster specs to catch errors early
type MockClient struct {
	shoots map[string]*mockShoot
	mu     sync.RWMutex
	logger *slog.Logger
	clock  func() time.Time // For testing, defaults to time.Now

	// For test assertions
	ApplyCalls    []ClusterToSync
	DeleteCalls   []ClusterToSync
	DeleteByName  []string
	ListCallCount int
	StatusCalls   []ClusterToSync

	// Configurable behavior for testing error paths
	ApplyError      error
	DeleteError     error
	ListError       error
	GetStatusError  error
	StatusOverrides map[uuid.UUID]StatusOverride // Per-cluster status override

	// Status progression timing (configurable for tests)
	ProgressingDelay time.Duration // Time before pending → progressing (default: 1s)
	ReadyDelay       time.Duration // Time before progressing → ready (default: 5s)
	DeleteDelay      time.Duration // Time before deleting → deleted (default: 3s)

	// Validation settings
	ValidateSpecs bool // Enable spec validation (default: true)
}

// mockShoot tracks a shoot's state and creation time for status progression.
type mockShoot struct {
	Info       ShootInfo
	CreatedAt  time.Time
	DeletedAt  *time.Time // Set when deletion starts
	Cluster    ClusterToSync
}

// StatusOverride allows tests to configure custom status for specific clusters.
type StatusOverride struct {
	Status  string
	Message string
}

// NewMock creates a new MockClient with default settings.
// Status progression is enabled with realistic delays.
// Spec validation is enabled to catch common errors.
func NewMock(logger *slog.Logger) *MockClient {
	return &MockClient{
		shoots:           make(map[string]*mockShoot),
		logger:           logger,
		clock:            time.Now,
		StatusOverrides:  make(map[uuid.UUID]StatusOverride),
		ProgressingDelay: 1 * time.Second,
		ReadyDelay:       5 * time.Second,
		DeleteDelay:      3 * time.Second,
		ValidateSpecs:    true,
	}
}

// NewMockInstant creates a MockClient with instant status transitions (no delays).
// Useful for unit tests that don't want to wait.
func NewMockInstant(logger *slog.Logger) *MockClient {
	m := NewMock(logger)
	m.ProgressingDelay = 0
	m.ReadyDelay = 0
	m.DeleteDelay = 0
	return m
}

// SetClock sets a custom clock function for testing time-based behavior.
func (m *MockClient) SetClock(clock func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clock = clock
}

// ApplyShoot records the call, validates the spec, and stores the shoot in memory.
func (m *MockClient) ApplyShoot(ctx context.Context, cluster *ClusterToSync) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ApplyCalls = append(m.ApplyCalls, *cluster)

	if m.ApplyError != nil {
		return m.ApplyError
	}

	// Validate spec if enabled
	if m.ValidateSpecs {
		if err := m.validateClusterSpec(cluster); err != nil {
			return fmt.Errorf("failed to create shoot: %w", err)
		}
	}

	shootName := ShootName(cluster.OrganizationName, cluster.Name, m.MaxShootNameLength())
	now := m.clock()

	// Check if shoot already exists (update case)
	if existing, exists := m.shoots[shootName]; exists {
		// Update preserves creation time
		existing.Cluster = *cluster
		m.logger.Info("MOCK: updated shoot", "shoot", shootName, "cluster_id", cluster.ID)
		return nil
	}

	// New shoot
	m.shoots[shootName] = &mockShoot{
		Info: ShootInfo{
			Name:      shootName,
			ClusterID: cluster.ID,
			Labels: map[string]string{
				"fundament.io/cluster-id":   cluster.ID.String(),
				"fundament.io/organization": cluster.OrganizationName,
			},
		},
		CreatedAt: now,
		Cluster:   *cluster,
	}
	m.logger.Info("MOCK: applied shoot", "shoot", shootName, "cluster_id", cluster.ID)
	return nil
}

// DeleteShoot records the call and marks the shoot for deletion.
// The shoot progresses through deleting → deleted status based on DeleteDelay.
func (m *MockClient) DeleteShoot(ctx context.Context, cluster *ClusterToSync) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteCalls = append(m.DeleteCalls, *cluster)

	if m.DeleteError != nil {
		return m.DeleteError
	}

	shootName := ShootName(cluster.OrganizationName, cluster.Name, m.MaxShootNameLength())
	now := m.clock()

	if shoot, exists := m.shoots[shootName]; exists {
		shoot.DeletedAt = &now
		m.logger.Info("MOCK: marked shoot for deletion", "shoot", shootName, "cluster_id", cluster.ID)
	} else {
		m.logger.Debug("MOCK: shoot already deleted", "shoot", shootName)
	}
	return nil
}

// DeleteShootByName records the call and marks the shoot for deletion by name.
func (m *MockClient) DeleteShootByName(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteByName = append(m.DeleteByName, name)

	if m.DeleteError != nil {
		return m.DeleteError
	}

	now := m.clock()
	if shoot, exists := m.shoots[name]; exists {
		shoot.DeletedAt = &now
		m.logger.Info("MOCK: marked shoot for deletion by name", "shoot", name)
	} else {
		m.logger.Debug("MOCK: shoot already deleted", "shoot", name)
	}
	return nil
}

// ListShoots returns all shoots stored in memory (excludes fully deleted ones).
func (m *MockClient) ListShoots(ctx context.Context) ([]ShootInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ListCallCount++

	if m.ListError != nil {
		return nil, m.ListError
	}

	now := m.clock()
	result := make([]ShootInfo, 0, len(m.shoots))
	for name, s := range m.shoots {
		// Skip fully deleted shoots
		if s.DeletedAt != nil && now.Sub(*s.DeletedAt) >= m.DeleteDelay {
			delete(m.shoots, name) // Clean up
			continue
		}
		result = append(result, s.Info)
	}
	return result, nil
}

// GetShootStatus returns the status of a shoot with realistic progression.
// Status progresses based on time elapsed since creation/deletion:
//   - pending → progressing (after ProgressingDelay)
//   - progressing → ready (after ReadyDelay)
//   - deleting → deleted (after DeleteDelay)
//
// Use StatusOverrides to customize per-cluster behavior for testing.
func (m *MockClient) GetShootStatus(ctx context.Context, cluster *ClusterToSync) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StatusCalls = append(m.StatusCalls, *cluster)

	if m.GetStatusError != nil {
		return "", "", m.GetStatusError
	}

	// Check for custom override (takes precedence)
	if override, ok := m.StatusOverrides[cluster.ID]; ok {
		return override.Status, override.Message, nil
	}

	shootName := ShootName(cluster.OrganizationName, cluster.Name, m.MaxShootNameLength())
	shoot, exists := m.shoots[shootName]
	if !exists {
		return StatusPending, MsgShootNotFound, nil
	}

	now := m.clock()

	// Handle deletion status progression
	if shoot.DeletedAt != nil {
		elapsed := now.Sub(*shoot.DeletedAt)
		if elapsed >= m.DeleteDelay {
			// Fully deleted - clean up and return deleted status
			delete(m.shoots, shootName)
			return StatusDeleted, "Shoot has been deleted", nil
		}
		return StatusDeleting, fmt.Sprintf("Shoot is being deleted (%.0fs remaining)",
			(m.DeleteDelay - elapsed).Seconds()), nil
	}

	// Handle creation status progression
	elapsed := now.Sub(shoot.CreatedAt)

	if elapsed < m.ProgressingDelay {
		return StatusPending, "Shoot creation initiated", nil
	}

	if elapsed < m.ProgressingDelay+m.ReadyDelay {
		progress := (elapsed - m.ProgressingDelay).Seconds() / m.ReadyDelay.Seconds() * 100
		if m.ReadyDelay == 0 {
			progress = 100
		}
		return StatusProgressing, fmt.Sprintf("Shoot is being created (%.0f%% complete)", progress), nil
	}

	return StatusReady, MsgShootReady, nil
}

// Reset clears all recorded calls and stored shoots.
// Useful for resetting state between test cases.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shoots = make(map[string]*mockShoot)
	m.ApplyCalls = nil
	m.DeleteCalls = nil
	m.DeleteByName = nil
	m.ListCallCount = 0
	m.StatusCalls = nil
	m.ApplyError = nil
	m.DeleteError = nil
	m.ListError = nil
	m.GetStatusError = nil
	m.StatusOverrides = make(map[uuid.UUID]StatusOverride)
}

// SetApplyError configures the mock to return an error on ApplyShoot.
func (m *MockClient) SetApplyError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ApplyError = err
}

// SetDeleteError configures the mock to return an error on DeleteShoot/DeleteShootByName.
func (m *MockClient) SetDeleteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeleteError = err
}

// SetStatusOverride configures a custom status for a specific cluster.
func (m *MockClient) SetStatusOverride(clusterID uuid.UUID, status, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StatusOverrides[clusterID] = StatusOverride{Status: status, Message: message}
}

// HasShoot checks if a shoot exists in the mock (excludes deleted shoots).
func (m *MockClient) HasShoot(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	shoot, exists := m.shoots[name]
	return exists && shoot.DeletedAt == nil
}

// ShootCount returns the number of active shoots in the mock (excludes deleted).
func (m *MockClient) ShootCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, s := range m.shoots {
		if s.DeletedAt == nil {
			count++
		}
	}
	return count
}

// MaxShootNameLength returns the max shoot name length (21 for local provider compatibility).
func (m *MockClient) MaxShootNameLength() int {
	return 21
}

// Verify MockClient implements Client interface.
var _ Client = (*MockClient)(nil)

// ErrMockApplyFailed is a sentinel error for testing apply failures.
var ErrMockApplyFailed = errors.New("mock: apply failed")

// ErrMockDeleteFailed is a sentinel error for testing delete failures.
var ErrMockDeleteFailed = errors.New("mock: delete failed")

// Validation errors (match Gardener's error format for realistic testing).
var (
	ErrInvalidVersion = errors.New("invalid semantic version")
	ErrEmptyRegion    = errors.New("region must not be empty")
	ErrEmptyName      = errors.New("name must not be empty")
	ErrInvalidName    = errors.New("name must match DNS label format")
)

// semverRegex matches semantic versions like "1.31.1", "1.32.0-rc.1".
var semverRegex = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$`)

// dnsLabelRegex matches valid DNS labels (lowercase alphanumeric, hyphens, max 63 chars).
var dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateClusterSpec validates a cluster spec like Gardener would.
// This catches common errors early in development/testing.
func (m *MockClient) validateClusterSpec(cluster *ClusterToSync) error {
	// Validate name
	if cluster.Name == "" {
		return fmt.Errorf("Shoot.core.gardener.cloud is invalid: metadata.name: %w", ErrEmptyName)
	}
	if len(cluster.Name) > 63 || !dnsLabelRegex.MatchString(cluster.Name) {
		return fmt.Errorf("Shoot.core.gardener.cloud is invalid: metadata.name: %w: %q", ErrInvalidName, cluster.Name)
	}

	// Validate region
	if cluster.Region == "" {
		return fmt.Errorf("Shoot.core.gardener.cloud is invalid: spec.region: %w", ErrEmptyRegion)
	}

	// Validate Kubernetes version (must be semver, not "1.31.x")
	if cluster.KubernetesVersion == "" {
		return fmt.Errorf("Shoot.core.gardener.cloud is invalid: spec.kubernetes.version: Required value")
	}
	if !semverRegex.MatchString(cluster.KubernetesVersion) {
		return fmt.Errorf("Shoot.core.gardener.cloud %q is invalid: failed to parse shoot version %q: %w",
			cluster.Name, cluster.KubernetesVersion, ErrInvalidVersion)
	}

	return nil
}
