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

func TestUserService_ListUsers_Populated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	want := []string{"List User A", "List User B", "List User C"}
	for _, name := range want {
		createUser(t, env, name, name+"@example.com", "")
	}

	resp, err := client.ListUsers(context.Background(), connect.NewRequest(&dcimv1.ListUsersRequest{}))
	require.NoError(t, err)

	got := make([]string, 0, len(resp.Msg.GetUsers()))
	for _, u := range resp.Msg.GetUsers() {
		got = append(got, u.GetName())
	}
	// ListUsers is unfiltered, so the response also includes seeded users (the
	// template DB is created with --insert-test-data). Assert the users we
	// created are present rather than expecting an exact match.
	assert.Subset(t, got, want)
}

// email is nullable, and userToProto only sets it when the column is non-null.
func TestUserService_ListUsers_NullEmail(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	userID := createUser(t, env, "No Email User", "", "")

	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE dcim.users SET email = NULL WHERE id = $1`, userID)
	require.NoError(t, err)

	resp, err := client.ListUsers(context.Background(), connect.NewRequest(&dcimv1.ListUsersRequest{}))
	require.NoError(t, err)

	var found *dcimv1.User
	for _, u := range resp.Msg.GetUsers() {
		if u.GetId() == userID {
			found = u
			break
		}
	}
	require.NotNil(t, found)

	assert.False(t, found.HasEmail())
}

// Soft-deleted users must drop out of the roster entirely.
func TestUserService_ListUsers_ExcludesSoftDeleted(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	userID := createUser(t, env, "Soft Deleted User", "gone@example.com", "")

	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE dcim.users SET deleted = now() WHERE id = $1`, userID)
	require.NoError(t, err)

	resp, err := client.ListUsers(context.Background(), connect.NewRequest(&dcimv1.ListUsersRequest{}))
	require.NoError(t, err)

	for _, u := range resp.Msg.GetUsers() {
		assert.NotEqual(t, userID, u.GetId())
	}
}
