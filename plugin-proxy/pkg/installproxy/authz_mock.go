package installproxy

import "context"

// MockAuthz allows every cluster view. Real-mode authz must re-check via
// OpenFGA.
type MockAuthz struct{}

func (MockAuthz) CanViewCluster(_ context.Context, _, _ string) (bool, error) {
	return true, nil
}
