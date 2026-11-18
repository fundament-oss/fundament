package authn

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
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

// Every functl and kubectl call exchanges a key. The exchange writes last_used
// at most once a minute, and no last_used write queues authz-worker work; a
// change that decides the key's tuples, such as a revocation, still does.
func TestLookupAPIKey_LastUsedQueuesNoAuthzWork(t *testing.T) {
	t.Parallel()

	env := newRefreshEnv(t)
	env.server.authz = &fakeAuthz{allow: true}
	ctx := context.Background()

	var orgID uuid.UUID
	require.NoError(t, env.superPool.QueryRow(ctx,
		`INSERT INTO tenant.organizations (name, alias) VALUES ('lastused', 'lastused') RETURNING id`).Scan(&orgID))
	userID := seedUser(t, env, "Agent", false)

	token, hash, err := apitoken.GenerateToken()
	require.NoError(t, err)
	var keyID uuid.UUID
	require.NoError(t, env.superPool.QueryRow(ctx,
		`INSERT INTO authn.api_keys (organization_id, user_id, name, token_hash, token_prefix)
		 VALUES ($1, $2, 'agent', $3, $4) RETURNING id`,
		orgID, userID, hash, apitoken.GetPrefix(token)).Scan(&keyID))

	outboxRows := func() int {
		var n int
		require.NoError(t, env.superPool.QueryRow(ctx,
			`SELECT count(*) FROM authz.outbox WHERE api_key_id = $1`, keyID).Scan(&n))
		return n
	}
	lastUsed := func() time.Time {
		var at time.Time
		require.NoError(t, env.superPool.QueryRow(ctx,
			`SELECT last_used FROM authn.api_keys WHERE id = $1`, keyID).Scan(&at))
		return at
	}
	afterInsert := outboxRows()

	_, err = env.server.lookupAPIKey(ctx, token)
	require.NoError(t, err)
	first := lastUsed()
	for range 4 {
		_, err := env.server.lookupAPIKey(ctx, token)
		require.NoError(t, err)
	}
	assert.Equal(t, first, lastUsed(), "exchanges within a minute keep the first last_used")
	assert.Equal(t, afterInsert, outboxRows(), "exchanges queue no authz work")

	_, err = env.superPool.Exec(ctx,
		`UPDATE authn.api_keys SET last_used = now() - interval '2 minutes' WHERE id = $1`, keyID)
	require.NoError(t, err)
	_, err = env.server.lookupAPIKey(ctx, token)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), lastUsed(), 10*time.Second, "an exchange after a minute writes last_used again")
	assert.Equal(t, afterInsert, outboxRows(), "neither last_used write queues authz work")

	_, err = env.superPool.Exec(ctx, `UPDATE authn.api_keys SET revoked = now() WHERE id = $1`, keyID)
	require.NoError(t, err)
	assert.Equal(t, afterInsert+1, outboxRows(), "a revocation queues the key for authz-worker")
}
