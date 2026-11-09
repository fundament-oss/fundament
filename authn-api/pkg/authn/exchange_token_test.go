package authn

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/apitoken"
)

// A well-formed key from another installation, or from before a database
// reset, is unknown rather than a server fault.
func TestLookupAPIKey_UnknownKey(t *testing.T) {
	t.Parallel()

	env := newRefreshEnv(t)

	token, _, err := apitoken.GenerateToken()
	require.NoError(t, err)

	_, err = env.server.lookupAPIKey(context.Background(), token)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
	assert.Equal(t, "invalid token", connectErr.Message())
}
