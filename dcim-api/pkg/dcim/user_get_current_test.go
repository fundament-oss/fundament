package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
)

func TestUserService_GetCurrentUser_Found(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	// Seed a decoy whose id does not match, so a lookup that ignores the
	// subject cannot pass by returning an arbitrary row.
	createUser(t, env, "Someone Else", "someone@example.com", "00000000-0000-0000-0000-0000000000ff")

	resp, err := client.GetCurrentUser(context.Background(),
		&dcimv1.GetCurrentUserRequest{},
	)
	require.NoError(t, err)

	user := resp.GetUser()
	require.NotNil(t, user)

	assert.Equal(t, env.subject, user.GetId())
	assert.Equal(t, callerName, user.GetName())
	assert.Equal(t, callerEmail, user.GetEmail())
}

// email is nullable, and userToProtoWithEmail only sets it when the column is
// non-null. GetCurrentUser is the only place the address is returned at all.
func TestUserService_GetCurrentUser_NullEmail(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE dcim.users SET email = NULL WHERE id = $1`, env.subject)
	require.NoError(t, err)

	resp, err := client.GetCurrentUser(context.Background(),
		&dcimv1.GetCurrentUserRequest{},
	)
	require.NoError(t, err)

	user := resp.GetUser()
	require.NotNil(t, user)

	assert.Equal(t, env.subject, user.GetId())
	assert.False(t, user.HasEmail())
}

// A soft-deleted user's still-valid token is refused at the interceptor: a
// signature alone is not a session, so revocation takes effect at the next
// RPC rather than at token expiry.
func TestUserService_GetCurrentUser_SoftDeleted(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE dcim.users SET deleted = now() WHERE id = $1`, env.subject)
	require.NoError(t, err)

	_, err = client.GetCurrentUser(context.Background(),
		&dcimv1.GetCurrentUserRequest{},
	)
	requireCode(t, err, connect.CodeUnauthenticated)
}
