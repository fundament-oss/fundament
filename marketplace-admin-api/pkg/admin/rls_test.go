package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests go around the server entirely and talk to Postgres as the admin
// role. The handler tests prove the service refuses; these prove the database
// refuses on its own: the role's column grants, not just the handlers' Go, are
// what fence the consent record.

func isGrantDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege
}

// The admin role sees every organization's submissions — that is its job.
func TestAdminRoleReadsAcrossOrganizations(t *testing.T) {
	env := newTestEnv(t)
	first := seedPendingSubmission(t, env, "cert-manager")
	second := seedPendingSubmission(t, env, "postgres-operator")

	var count int
	err := env.servicePool.QueryRow(context.Background(),
		`SELECT count(*) FROM appstore.submissions WHERE id = ANY($1)`,
		[]any{first.submissionID, second.submissionID}).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// hash and manifest are the consent record (FUN-17): the reviewer's role holds
// no UPDATE grant on them, so even a compromised handler cannot rewrite what
// an install pins against.
func TestAdminRoleCannotRewriteConsentRecord(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	_, err := env.servicePool.Exec(context.Background(),
		`UPDATE appstore.plugin_definitions SET hash = 'sha256:forged' WHERE id = $1`, seeded.versionID)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)

	_, err = env.servicePool.Exec(context.Background(),
		`UPDATE appstore.plugin_definitions SET manifest = '\x00' WHERE id = $1`, seeded.versionID)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)
}

// A round's identity — which version, who submitted — is the publisher's
// alone.
func TestAdminRoleCannotRewriteSubmissionIdentity(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")
	other := seedPendingSubmission(t, env, "postgres-operator")

	_, err := env.servicePool.Exec(context.Background(),
		`UPDATE appstore.submissions SET plugin_definition_id = $1 WHERE id = $2`,
		other.versionID, seeded.submissionID)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)
}

// The reviewer reads listings but does not author them.
func TestAdminRoleCannotWriteListings(t *testing.T) {
	env := newTestEnv(t)
	seeded := seedPendingSubmission(t, env, "cert-manager")

	_, err := env.servicePool.Exec(context.Background(),
		`UPDATE appstore.plugins SET display_name = 'renamed' WHERE id = $1`, seeded.pluginID)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)

	_, err = env.servicePool.Exec(context.Background(),
		`INSERT INTO appstore.plugins (organization_id, name, display_name, description, visibility)
		 VALUES ($1, 'rogue', 'rogue', '', 'public')`, seeded.organizationID)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)
}

// The role reaches tenant.organizations only through its publisher policy;
// the rest of the tenant schema stays out of reach.
func TestAdminRoleCannotReadUsers(t *testing.T) {
	env := newTestEnv(t)

	var count int
	err := env.servicePool.QueryRow(context.Background(),
		`SELECT count(*) FROM tenant.users`).Scan(&count)
	require.Error(t, err)
	assert.True(t, isGrantDenied(err), "expected insufficient_privilege, got: %v", err)
}

// An organization with no listing is invisible even to the backoffice: the
// publisher policy grants publishers, not tenants.
func TestAdminRoleSeesOnlyPublishers(t *testing.T) {
	env := newTestEnv(t)
	publisher := seedPendingSubmission(t, env, "cert-manager")
	bystander := seedOrganization(t, env, "no-listings")

	rows, err := env.servicePool.Query(context.Background(),
		`SELECT id FROM tenant.organizations WHERE id = ANY($1)`,
		[]any{publisher.organizationID, bystander})
	require.NoError(t, err)
	defer rows.Close()

	var seen []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		seen = append(seen, id)
	}
	require.NoError(t, rows.Err())

	assert.Contains(t, seen, publisher.organizationID.String())
	assert.NotContains(t, seen, bystander.String())
}
