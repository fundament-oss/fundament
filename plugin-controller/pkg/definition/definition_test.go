package definition

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockResolver_ResolvesKnownHash(t *testing.T) {
	r := NewMockResolver()
	def, err := r.Resolve(context.Background(), "sha256:mock")
	require.NoError(t, err)
	require.NotEmpty(t, def.Permissions.RBAC, "expected at least one RBAC rule in the mock definition")
	assert.NotEmpty(t, def.Permissions.RBAC[0].Verbs)
}

func TestMockResolver_UnknownHashErrors(t *testing.T) {
	r := NewMockResolver()
	_, err := r.Resolve(context.Background(), "sha256:does-not-exist")
	require.Error(t, err, "expected error for unknown definition hash")
}
