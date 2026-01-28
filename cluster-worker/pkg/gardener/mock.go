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

// MockEvent represents an event in the mock client's history.
type MockEvent struct {
	Time      time.Time
	Type      string // "apply", "delete", "status_change"
	ClusterID uuid.UUID
	ShootName string
	Status    string // For status_change events
	Message   string
}

// MockClient implements Client for testing.
// It stores shoots in-memory and tracks all calls for test assertions.
// Features:
//   - Status progression: shoots progress through pending → progressing → ready
//   - Spec validation: validates cluster specs to catch errors early
//   - Event history: tracks all operations for debugging
type MockClient struct {
	shoots   map[string]*mockShoot
	projects map[string]bool // Tracks created projects by name
	mu       sync.RWMutex
	logger   *slog.Logger
	clock    func() time.Time // For testing, defaults to time.Now

	// For test assertions
	EnsureProjectCalls []string // Project names
	ApplyCalls         []ClusterToSync
	DeleteByClusterID  []uuid.UUID
	ListCallCount      int
	StatusCalls        []ClusterToSync

	// Event history for debugging (visible via GetEventHistory)
	EventHistory []MockEvent

	// Configurable behavior for testing error paths
	EnsureProjectError error
	ApplyError         error
	DeleteError        error
	ListError          error
	GetStatusError     error
	StatusOverrides    map[uuid.UUID]StatusOverride // Per-cluster status override

	// Status progression timing (configurable for tests)
	ProgressingDelay time.Duration // Time before pending → progressing (default: 1s)
	ReadyDelay       time.Duration // Time before progressing → ready (default: 5s)
	DeleteDelay      time.Duration // Time before deleting → deleted (default: 3s)

	// Validation settings
	ValidateSpecs bool // Enable spec validation (default: true)

	// Async namespace simulation (like real Gardener)
	SimulateAsyncNamespace bool                    // If true, first EnsureProject call returns empty namespace
	projectCreatedAt       map[string]time.Time    // Track when projects were created
	NamespaceReadyDelay    time.Duration           // Time before namespace becomes ready (default: 2s)
}

// mockShoot tracks a shoot's state and creation time for status progression.
type mockShoot struct {
	Info       ShootInfo
	CreatedAt  time.Time
	DeletedAt  *time.Time // Set when deletion starts
	Cluster    ClusterToSync
	LastStatus ShootStatusType // Track last status for change detection
}

// StatusOverride allows tests to configure custom status for specific clusters.
type StatusOverride struct {
	Status  ShootStatusType
	Message string
}

// NewMock creates a new MockClient with default settings.
// Status progression is enabled with realistic delays.
// Spec validation is enabled to catch common errors.
func NewMock(logger *slog.Logger) *MockClient {
	return &MockClient{
		shoots:                 make(map[string]*mockShoot),
		projects:               make(map[string]bool),
		projectCreatedAt:       make(map[string]time.Time),
		logger:                 logger,
		clock:                  time.Now,
		StatusOverrides:        make(map[uuid.UUID]StatusOverride),
		ProgressingDelay:       1 * time.Second,
		ReadyDelay:             5 * time.Second,
		DeleteDelay:            3 * time.Second,
		ValidateSpecs:          true,
		SimulateAsyncNamespace: false,             // Default: instant namespace (backwards compatible)
		NamespaceReadyDelay:    2 * time.Second,
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

// EnsureProject records the call and creates the project if it doesn't exist (idempotent).
// Returns the namespace (garden-{projectName} for mock).
// If SimulateAsyncNamespace is enabled, returns empty namespace on first call (like real Gardener).
func (m *MockClient) EnsureProject(ctx context.Context, projectName string, orgID uuid.UUID) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.EnsureProjectCalls = append(m.EnsureProjectCalls, projectName)

	if m.EnsureProjectError != nil {
		return "", m.EnsureProjectError
	}

	namespace := NamespaceFromProjectName(projectName)
	now := m.clock()

	if !m.projects[projectName] {
		m.projects[projectName] = true
		m.projectCreatedAt[projectName] = now
		m.logger.Info("MOCK: created project", "project", projectName, "namespace", namespace, "organization_id", orgID)

		// Simulate async namespace creation like real Gardener
		if m.SimulateAsyncNamespace {
			m.logger.Debug("MOCK: namespace not ready yet (async simulation)", "project", projectName)
			return "", nil
		}
	}

	// Check if namespace is ready (for async simulation)
	if m.SimulateAsyncNamespace {
		createdAt, exists := m.projectCreatedAt[projectName]
		if exists && now.Sub(createdAt) < m.NamespaceReadyDelay {
			m.logger.Debug("MOCK: namespace not ready yet", "project", projectName, "elapsed", now.Sub(createdAt))
			return "", nil
		}
	}

	return namespace, nil
}

// ApplyShoot records the call, validates the spec, and stores the shoot in memory.
// Requires cluster.ShootName to be set (generated at cluster creation time by API).
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

	shootName := cluster.ShootName
	if shootName == "" {
		return fmt.Errorf("shoot name is required (must be generated at cluster creation time)")
	}

	now := m.clock()

	// Check if shoot already exists (update case)
	if existing, exists := m.shoots[shootName]; exists {
		// Update preserves creation time
		existing.Cluster = *cluster

		// Record event
		m.EventHistory = append(m.EventHistory, MockEvent{
			Time:      now,
			Type:      "apply",
			ClusterID: cluster.ID,
			ShootName: shootName,
			Message:   "Shoot updated",
		})

		m.logger.Info("MOCK: updated shoot", "shoot", shootName, "cluster_id", cluster.ID)
		return nil
	}

	// New shoot
	m.shoots[shootName] = &mockShoot{
		Info: ShootInfo{
			Name:      shootName,
			Namespace: cluster.Namespace,
			ClusterID: cluster.ID,
			Labels: map[string]string{
				LabelClusterID:      cluster.ID.String(),
				LabelOrganizationID: cluster.OrganizationID.String(),
			},
		},
		CreatedAt: now,
		Cluster:   *cluster,
	}

	// Record event
	m.EventHistory = append(m.EventHistory, MockEvent{
		Time:      now,
		Type:      "apply",
		ClusterID: cluster.ID,
		ShootName: shootName,
		Message:   "Shoot created",
	})

	m.logger.Info("MOCK: applied shoot", "shoot", shootName, "cluster_id", cluster.ID)
	return nil
}

// DeleteShootByClusterID records the call and marks the shoot for deletion by cluster ID.
func (m *MockClient) DeleteShootByClusterID(ctx context.Context, clusterID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteByClusterID = append(m.DeleteByClusterID, clusterID)

	if m.DeleteError != nil {
		return m.DeleteError
	}

	// Find shoot by cluster ID
	now := m.clock()
	for name, shoot := range m.shoots {
		if shoot.Info.ClusterID == clusterID {
			shoot.DeletedAt = &now
			m.logger.Info("MOCK: marked shoot for deletion by cluster ID", "shoot", name, "cluster_id", clusterID)
			return nil
		}
	}

	m.logger.Debug("MOCK: shoot not found for deletion by cluster ID", "cluster_id", clusterID)
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
// Records status_change events in EventHistory when status transitions.
// Use StatusOverrides to customize per-cluster behavior for testing.
func (m *MockClient) GetShootStatus(ctx context.Context, cluster *ClusterToSync) (*ShootStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StatusCalls = append(m.StatusCalls, *cluster)

	if m.GetStatusError != nil {
		return nil, m.GetStatusError
	}

	// Check for custom override (takes precedence)
	if override, ok := m.StatusOverrides[cluster.ID]; ok {
		return &ShootStatus{Status: override.Status, Message: override.Message}, nil
	}

	// Look up shoot by cluster ID (not shoot name)
	shoot, shootName := m.findShootByClusterID(cluster.ID)
	if shoot == nil {
		return &ShootStatus{Status: StatusPending, Message: MsgShootNotFound}, nil
	}

	now := m.clock()
	var status *ShootStatus

	// Handle deletion status progression
	if shoot.DeletedAt != nil {
		elapsed := now.Sub(*shoot.DeletedAt)
		if elapsed >= m.DeleteDelay {
			// Fully deleted - clean up and return deleted status
			delete(m.shoots, shootName)
			return &ShootStatus{Status: StatusPending, Message: MsgShootNotFound}, nil
		}
		status = &ShootStatus{
			Status:  StatusDeleting,
			Message: fmt.Sprintf("Shoot is being deleted (%.0fs remaining)", (m.DeleteDelay - elapsed).Seconds()),
		}
	} else {
		// Handle creation status progression
		elapsed := now.Sub(shoot.CreatedAt)

		if elapsed < m.ProgressingDelay {
			status = &ShootStatus{Status: StatusPending, Message: "Shoot creation initiated"}
		} else if elapsed < m.ProgressingDelay+m.ReadyDelay {
			progress := (elapsed - m.ProgressingDelay).Seconds() / m.ReadyDelay.Seconds() * 100
			if m.ReadyDelay == 0 {
				progress = 100
			}
			status = &ShootStatus{
				Status:  StatusProgressing,
				Message: fmt.Sprintf("Shoot is being created (%.0f%% complete)", progress),
			}
		} else {
			status = &ShootStatus{Status: StatusReady, Message: MsgShootReady}
		}
	}

	// Record status_change event if status changed
	if shoot.LastStatus != status.Status {
		m.EventHistory = append(m.EventHistory, MockEvent{
			Time:      now,
			Type:      "status_change",
			ClusterID: cluster.ID,
			ShootName: shootName,
			Status:    string(status.Status),
			Message:   status.Message,
		})
		shoot.LastStatus = status.Status
		m.logger.Debug("MOCK: status changed",
			"cluster_id", cluster.ID,
			"shoot", shootName,
			"old_status", shoot.LastStatus,
			"new_status", status.Status)
	}

	return status, nil
}

// SetClock sets a custom clock function for testing time-based behavior.
func (m *MockClient) SetClock(clock func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clock = clock
}

// Reset clears all recorded calls, events, and stored shoots.
// Useful for resetting state between test cases.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shoots = make(map[string]*mockShoot)
	m.projects = make(map[string]bool)
	m.projectCreatedAt = make(map[string]time.Time)
	m.EnsureProjectCalls = nil
	m.ApplyCalls = nil
	m.DeleteByClusterID = nil
	m.ListCallCount = 0
	m.StatusCalls = nil
	m.EventHistory = nil
	m.EnsureProjectError = nil
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

// SetDeleteError configures the mock to return an error on DeleteShoot/DeleteShootByClusterID.
func (m *MockClient) SetDeleteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeleteError = err
}

// SetStatusOverride configures a custom status for a specific cluster.
func (m *MockClient) SetStatusOverride(clusterID uuid.UUID, status ShootStatusType, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StatusOverrides[clusterID] = StatusOverride{Status: status, Message: message}
}

// HasShootForCluster checks if a shoot exists for the given cluster ID (excludes deleted shoots).
func (m *MockClient) HasShootForCluster(clusterID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	shoot, _ := m.findShootByClusterID(clusterID)
	return shoot != nil && shoot.DeletedAt == nil
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

// GetEventHistory returns a copy of all recorded events.
// Useful for debugging and test assertions.
func (m *MockClient) GetEventHistory() []MockEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	events := make([]MockEvent, len(m.EventHistory))
	copy(events, m.EventHistory)
	return events
}

// GetEventHistoryForCluster returns events for a specific cluster.
func (m *MockClient) GetEventHistoryForCluster(clusterID uuid.UUID) []MockEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var events []MockEvent
	for _, e := range m.EventHistory {
		if e.ClusterID == clusterID {
			events = append(events, e)
		}
	}
	return events
}

// Verify MockClient implements Client interface.
var _ Client = (*MockClient)(nil)

// findShootByClusterID finds a shoot by cluster ID.
// Must be called with lock held.
func (m *MockClient) findShootByClusterID(clusterID uuid.UUID) (*mockShoot, string) {
	for name, shoot := range m.shoots {
		if shoot.Cluster.ID == clusterID {
			return shoot, name
		}
	}
	return nil, ""
}

// semverRegex matches semantic versions like "1.31.1", "1.32.0-rc.1".
var semverRegex = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`)

// dnsLabelRegex matches valid DNS labels (lowercase alphanumeric, hyphens, max 63 chars).
var dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateClusterSpec validates a cluster spec like Gardener would.
// This catches common errors early in development/testing.
func (m *MockClient) validateClusterSpec(cluster *ClusterToSync) error {
	// Validate name
	if cluster.Name == "" {
		return fmt.Errorf("shoot.core.gardener.cloud is invalid: metadata.name: %w", ErrEmptyName)
	}
	if len(cluster.Name) > 63 || !dnsLabelRegex.MatchString(cluster.Name) {
		return fmt.Errorf("shoot.core.gardener.cloud is invalid: metadata.name: %w: %q", ErrInvalidName, cluster.Name)
	}

	// Validate region
	if cluster.Region == "" {
		return fmt.Errorf("shoot.core.gardener.cloud is invalid: spec.region: %w", ErrEmptyRegion)
	}

	// Validate Kubernetes version (must be semver, not "1.31.x")
	if cluster.KubernetesVersion == "" {
		return fmt.Errorf("shoot.core.gardener.cloud is invalid: spec.kubernetes.version: required value")
	}
	if !semverRegex.MatchString(cluster.KubernetesVersion) {
		return fmt.Errorf("shoot.core.gardener.cloud %q is invalid: failed to parse shoot version %q: %w",
			cluster.Name, cluster.KubernetesVersion, ErrInvalidVersion)
	}

	return nil
}

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
