package dcim_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The JWT subject is an identity-provider reference, not a DCIM user id, so the
// entry has to be found via external_ref and hand back the internal id that
// tasks are assigned to.
func TestUserService_GetCurrentUser_Found(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	// Seed a decoy whose external_ref does not match, so a lookup that ignores
	// the subject cannot pass by returning an arbitrary row.
	createUser(t, env, "Someone Else", "someone@example.com", "00000000-0000-0000-0000-0000000000ff")
	userID := createUser(t, env, "Current User", "current@example.com", env.subject)

	resp, err := client.GetCurrentUser(context.Background(), connect.NewRequest(
		&dcimv1.GetCurrentUserRequest{},
	))
	require.NoError(t, err)

	user := resp.Msg.GetUser()
	require.NotNil(t, user)

	assert.Equal(t, userID, user.GetId())
	assert.Equal(t, "Current User", user.GetName())
	assert.Equal(t, "current@example.com", user.GetEmail())
}

// email is nullable, and userToProtoWithEmail only sets it when the column is
// non-null. GetCurrentUser is the only place the address is returned at all.
func TestUserService_GetCurrentUser_NullEmail(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	userID := createUser(t, env, "No Email User", "", env.subject)

	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE dcim.users SET email = NULL WHERE id = $1`, userID)
	require.NoError(t, err)

	resp, err := client.GetCurrentUser(context.Background(), connect.NewRequest(
		&dcimv1.GetCurrentUserRequest{},
	))
	require.NoError(t, err)

	user := resp.Msg.GetUser()
	require.NotNil(t, user)

	assert.Equal(t, userID, user.GetId())
	assert.False(t, user.HasEmail())
}

// A valid token for someone with no directory entry is a 404, not a 500 — the
// caller is authenticated, they just are not in the DCIM roster.
func TestUserService_GetCurrentUser_NotFound(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	_, err := client.GetCurrentUser(context.Background(), connect.NewRequest(
		&dcimv1.GetCurrentUserRequest{},
	))
	requireCode(t, err, connect.CodeNotFound)
}

// A soft-deleted entry must read as absent: the roster query filters on
// deleted IS NULL, and the unique index on external_ref is partial to match.
func TestUserService_GetCurrentUser_SoftDeleted(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	userID := createUser(t, env, "Deleted User", "deleted@example.com", env.subject)

	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE dcim.users SET deleted = now() WHERE id = $1`, userID)
	require.NoError(t, err)

	_, err = client.GetCurrentUser(context.Background(), connect.NewRequest(
		&dcimv1.GetCurrentUserRequest{},
	))
	requireCode(t, err, connect.CodeNotFound)
}
