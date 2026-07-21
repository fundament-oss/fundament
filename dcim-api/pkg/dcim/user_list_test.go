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

// The roster is readable by every authenticated caller, so it must not hand out
// the staff directory's email addresses — not even for users that have one. The
// caller reads their own address through GetCurrentUser instead.
func TestUserService_ListUsers_OmitsEmail(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)
	client := dcimv1connect.NewUserServiceClient(env.client(), env.server.URL)

	userID := createUser(t, env, "Emailed User", "listed@example.com", "")

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

	assert.Equal(t, "Emailed User", found.GetName())
	assert.False(t, found.HasEmail(), "the roster listing must not expose email addresses")

	for _, u := range resp.Msg.GetUsers() {
		assert.False(t, u.HasEmail(), "no roster entry may carry an email")
	}
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
