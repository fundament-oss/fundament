package fleet_test

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

func TestFleetRequiresStaffToken(t *testing.T) {
	env := testpg.NewEnv(t, testDBPort)

	t.Run("no token", func(t *testing.T) {
		_, err := newClient(t, env).GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("console token", func(t *testing.T) {
		token := mintToken(t, auth.ConsoleIssuer, jwt.ClaimStrings{auth.TokenTypeUser})
		_, err := newClient(t, env, bearer(token)).GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "a customer's console token is refused by issuer")
	})

	t.Run("DCIM issuer without the staff audience", func(t *testing.T) {
		// Any service holding the shared secret could sign this; only
		// dcim-authn-api mints the dcim-user audience.
		token := mintToken(t, auth.DCIMIssuer, nil)
		_, err := newClient(t, env, bearer(token)).GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("token signed with another secret", func(t *testing.T) {
		token := mintTokenWithSecret(t, []byte("not-the-secret"), auth.DCIMIssuer, jwt.ClaimStrings{auth.TokenTypeDCIMUser})
		_, err := newClient(t, env, bearer(token)).GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("staff token", func(t *testing.T) {
		_, err := staffClient(t, env).GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
		require.NoError(t, err)
	})
}
