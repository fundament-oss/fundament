package installproxy

import (
	"context"
	"fmt"
)

// Authz enforces OpenFGA can_view on (user, cluster). Stubbed until the
// OpenFGA wiring lands.
type Authz struct{}

func (Authz) CanViewCluster(_ context.Context, _, _ string) (bool, error) {
	return false, fmt.Errorf("cluster authz not yet wired")
}
