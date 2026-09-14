package authn

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/psqldb"
)

// loginEnv is an AuthnServer wired to a throwaway database, with an admin pool
// for arranging and inspecting rows the way the organization-api would.
type loginEnv struct {
	server    *AuthnServer
	adminPool *pgxpool.Pool
}

func newLoginEnv(t *testing.T) *loginEnv {
	t.Helper()

	name := testNameToDbName(t.Name())
	createTestDatabase(t, name)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	database, err := psqldb.New(t.Context(), logger, psqldb.Config{
		URL: fmt.Sprintf("postgres://fun_authn_api@localhost:%d/%s?sslmode=disable", testDBPort, name),
	})
	require.NoError(t, err)
	t.Cleanup(database.Close)

	server, err := New(logger, &Config{
		TokenExpiry: time.Hour,
		JWTSecret:   []byte(uuid.New().String()),
	}, nil, nil, nil, database, nil, nil)
	require.NoError(t, err)

	adminPool, err := pgxpool.New(t.Context(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, name))
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	return &loginEnv{server: server, adminPool: adminPool}
}

func createTestDatabase(t *testing.T, name string) {
	t.Helper()

	adminURL := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", testDBPort)

	adminPool, err := pgxpool.New(context.Background(), adminURL)
	require.NoError(t, err)
	defer adminPool.Close()

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	require.NoError(t, err)
}

// inviteMember writes exactly what organization-api's InviteMember writes: a
// user row carrying only the email address, plus a pending membership.
func (e *loginEnv) inviteMember(t *testing.T, organizationID uuid.UUID, email string) uuid.UUID {
	t.Helper()

	var userID uuid.UUID
	err := e.adminPool.QueryRow(t.Context(),
		"INSERT INTO tenant.users (name, email) VALUES ($1, $1) RETURNING id", email,
	).Scan(&userID)
	require.NoError(t, err)

	_, err = e.adminPool.Exec(t.Context(),
		"INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status) VALUES ($1, $2, 'viewer', 'pending')",
		organizationID, userID)
	require.NoError(t, err)

	return userID
}

func (e *loginEnv) createOrganization(t *testing.T, name string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := e.adminPool.QueryRow(t.Context(),
		"INSERT INTO tenant.organizations (name, alias) VALUES ($1, $1) RETURNING id", name,
	).Scan(&id)
	require.NoError(t, err)

	return id
}

func (e *loginEnv) organizationNames(t *testing.T) []string {
	t.Helper()

	rows, err := e.adminPool.Query(t.Context(), "SELECT name FROM tenant.organizations ORDER BY created")
	require.NoError(t, err)
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())

	return names
}

// Test_ProcessOIDCLogin_InvitedUserJoinsInvitingOrganization is the whole point
// of an invitation: the person who accepts it ends up in the organization that
// invited them, not in one of their own.
func Test_ProcessOIDCLogin_InvitedUserJoinsInvitingOrganization(t *testing.T) {
	env := newLoginEnv(t)

	orgID := env.createOrganization(t, "acme")
	invitedUserID := env.inviteMember(t, orgID, "invitee@example.com")

	before := env.organizationNames(t)

	u, _, err := env.server.processOIDCLogin(t.Context(), &oidcClaims{
		Sub:   "CgdpbnZpdGVlEgVsb2NhbA",
		Email: "invitee@example.com",
		Name:  "Invitee",
	}, "oidc")
	require.NoError(t, err)

	assert.Equal(t, invitedUserID, u.ID, "login must claim the invited user row")
	assert.Equal(t, before, env.organizationNames(t), "login must not create an organization")
}

// Test_ProcessOIDCLogin_InvitedUserMixedCaseEmail covers the same thing for an
// address typed with different capitalisation than the identity provider
// reports, which is the same address as far as anybody inviting is concerned.
func Test_ProcessOIDCLogin_InvitedUserMixedCaseEmail(t *testing.T) {
	env := newLoginEnv(t)

	orgID := env.createOrganization(t, "acme")
	invitedUserID := env.inviteMember(t, orgID, "Invitee@Example.com")

	before := env.organizationNames(t)

	u, _, err := env.server.processOIDCLogin(t.Context(), &oidcClaims{
		Sub:   "CgdpbnZpdGVlEgVsb2NhbA",
		Email: "invitee@example.com",
		Name:  "Invitee",
	}, "oidc")
	require.NoError(t, err)

	assert.Equal(t, invitedUserID, u.ID, "login must claim the invited user row")
	assert.Equal(t, before, env.organizationNames(t), "login must not create an organization")
}

// Test_ProcessOIDCLogin_UninvitedUserGetsOwnOrganization is the other side of
// it: somebody nobody invited still gets an organization to start in.
func Test_ProcessOIDCLogin_UninvitedUserGetsOwnOrganization(t *testing.T) {
	env := newLoginEnv(t)

	before := env.organizationNames(t)

	u, _, err := env.server.processOIDCLogin(t.Context(), &oidcClaims{
		Sub:   "CghzdHJhbmdlchIFbG9jYWw",
		Email: "stranger@example.com",
		Name:  "Stranger",
	}, "oidc")
	require.NoError(t, err)

	require.Len(t, u.OrganizationIDs, 1)
	assert.Len(t, env.organizationNames(t), len(before)+1)
}
