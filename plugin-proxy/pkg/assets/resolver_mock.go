package assets

import (
	"context"

	"github.com/google/uuid"
)

// MockResolver pins every (pluginName, version) lookup to a fixed clusterID
// so the dev iframe can render without a live cluster.
type MockResolver struct {
	ClusterID uuid.UUID
}

func (m MockResolver) ClusterFor(_ context.Context, _, _ string) (uuid.UUID, error) {
	return m.ClusterID, nil
}
