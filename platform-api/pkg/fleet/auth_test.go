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

	t.Run("token signed with another secret", func(t *testing.T) {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Issuer: auth.DCIMIssuer, Subject: "019b4000-1000-7000-8000-000000000001"},
		}).SignedString([]byte("not-the-secret"))
		require.NoError(t, err)
		_, err = newClient(t, env, bearer(token)).GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("staff token", func(t *testing.T) {
		_, err := staffClient(t, env).GetFleetSummary(t.Context(), &fleetv1.GetFleetSummaryRequest{})
		require.NoError(t, err)
	})
}
