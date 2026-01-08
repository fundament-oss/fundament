package gardener

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

// MockClient implements Client for testing.
// It stores shoots in-memory and tracks all calls for test assertions.
type MockClient struct {
	shoots map[string]ShootInfo
	mu     sync.RWMutex
	logger *slog.Logger

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
}

// StatusOverride allows tests to configure custom status for specific clusters.
type StatusOverride struct {
	Status  string
	Message string
}

// NewMock creates a new MockClient.
func NewMock(logger *slog.Logger) *MockClient {
	return &MockClient{
		shoots:          make(map[string]ShootInfo),
		logger:          logger,
		StatusOverrides: make(map[uuid.UUID]StatusOverride),
	}
}

// ApplyShoot records the call and stores the shoot in memory.
func (m *MockClient) ApplyShoot(ctx context.Context, cluster ClusterToSync) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ApplyCalls = append(m.ApplyCalls, cluster)

	if m.ApplyError != nil {
		return m.ApplyError
	}

	shootName := ShootName(cluster.TenantName, cluster.Name)
	m.shoots[shootName] = ShootInfo{
		Name:      shootName,
		ClusterID: cluster.ID,
		Labels: map[string]string{
			"fundament.io/cluster-id": cluster.ID.String(),
			"fundament.io/tenant":     cluster.TenantName,
		},
	}
	m.logger.Info("MOCK: applied shoot", "shoot", shootName, "cluster_id", cluster.ID)
	return nil
}

// DeleteShoot records the call and removes the shoot from memory.
func (m *MockClient) DeleteShoot(ctx context.Context, cluster ClusterToSync) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteCalls = append(m.DeleteCalls, cluster)

	if m.DeleteError != nil {
		return m.DeleteError
	}

	shootName := ShootName(cluster.TenantName, cluster.Name)
	delete(m.shoots, shootName)
	m.logger.Info("MOCK: deleted shoot", "shoot", shootName, "cluster_id", cluster.ID)
	return nil
}

// DeleteShootByName records the call and removes the shoot by name.
func (m *MockClient) DeleteShootByName(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteByName = append(m.DeleteByName, name)

	if m.DeleteError != nil {
		return m.DeleteError
	}

	delete(m.shoots, name)
	m.logger.Info("MOCK: deleted shoot by name", "shoot", name)
	return nil
}

// ListShoots returns all shoots stored in memory.
func (m *MockClient) ListShoots(ctx context.Context) ([]ShootInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.ListCallCount++

	if m.ListError != nil {
		return nil, m.ListError
	}

	result := make([]ShootInfo, 0, len(m.shoots))
	for _, s := range m.shoots {
		result = append(result, s)
	}
	return result, nil
}

// GetShootStatus returns the status of a shoot.
// By default returns "ready" for existing shoots and "pending" (not found) for non-existing.
// Use StatusOverrides to customize per-cluster behavior.
func (m *MockClient) GetShootStatus(ctx context.Context, cluster ClusterToSync) (string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.StatusCalls = append(m.StatusCalls, cluster)

	if m.GetStatusError != nil {
		return "", "", m.GetStatusError
	}

	// Check for custom override
	if override, ok := m.StatusOverrides[cluster.ID]; ok {
		return override.Status, override.Message, nil
	}

	shootName := ShootName(cluster.TenantName, cluster.Name)
	if _, exists := m.shoots[shootName]; exists {
		return "ready", "Mock shoot is ready", nil
	}
	return "pending", "Shoot not found in Gardener", nil
}

// Reset clears all recorded calls and stored shoots.
// Useful for resetting state between test cases.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shoots = make(map[string]ShootInfo)
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

// HasShoot checks if a shoot exists in the mock.
func (m *MockClient) HasShoot(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.shoots[name]
	return exists
}

// ShootCount returns the number of shoots in the mock.
func (m *MockClient) ShootCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.shoots)
}

// Verify MockClient implements Client interface.
var _ Client = (*MockClient)(nil)

// ErrMockApplyFailed is a sentinel error for testing apply failures.
var ErrMockApplyFailed = errors.New("mock: apply failed")

// ErrMockDeleteFailed is a sentinel error for testing delete failures.
var ErrMockDeleteFailed = errors.New("mock: delete failed")
