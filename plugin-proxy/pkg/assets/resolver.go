package assets

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Resolver maps (pluginName, version) to the cluster currently running it.
// Stubbed until the installation lookup is wired.
type Resolver struct{}

func (Resolver) ClusterFor(_ context.Context, _, _ string) (uuid.UUID, error) {
	return uuid.Nil, fmt.Errorf("plugin cluster resolver not yet wired")
}
