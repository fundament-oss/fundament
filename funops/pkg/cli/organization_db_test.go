package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Organizations are soft-deleted: the row stays with deleted set, what hung
// off it is revoked, and the commands treat it as gone.

// organizationRows counts the rows for a name, live and in total.
func organizationRows(t *testing.T, ctx *Context, name string) (live, all int) {
	t.Helper()

	err := ctx.DB.Pool.QueryRow(t.Context(),
		`SELECT count(*) FILTER (WHERE deleted IS NULL), count(*) FROM tenant.organizations WHERE name = $1`, name).Scan(&live, &all)
	require.NoError(t, err)
	return live, all
}

func listedOrganizationNames(t *testing.T, ctx *Context) []string {
	t.Helper()

	rows, err := ctx.Queries.OrganizationList(t.Context())
	require.NoError(t, err)

	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.Name
	}
	return names
}

func TestOrganizationDelete_SoftDeletes(t *testing.T) {
	ctx := newTestContext(t)

	require.NoError(t, (&OrganizationCreateCmd{Name: "short-lived"}).Run(ctx))
	require.NoError(t, (&OrganizationDeleteCmd{Name: "short-lived"}).Run(ctx))

	live, all := organizationRows(t, ctx, "short-lived")
	assert.Equal(t, 0, live)
	assert.Equal(t, 1, all, "the deleted organization is kept")
	assert.NotContains(t, listedOrganizationNames(t, ctx), "short-lived")

	// The name is free again.
	require.NoError(t, (&OrganizationCreateCmd{Name: "short-lived"}).Run(ctx))
}

func TestOrganizationDelete_RevokesMembershipsAndAPIKeys(t *testing.T) {
	ctx := newTestContext(t)

	// Globex has members and no clusters. David is one of them.
	_, err := ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO authn.api_keys (organization_id, user_id, name, token_hash, token_prefix)
		VALUES ('019b4000-0000-7000-8000-000000000002', '019b4000-1000-7000-8000-000000000004', 'ci', '\x01', 'fun_test')`)
	require.NoError(t, err)

	require.NoError(t, (&OrganizationDeleteCmd{Name: "globex"}).Run(ctx))

	var liveMemberships, revokedMemberships, liveKeys int
	err = ctx.DB.Pool.QueryRow(t.Context(), `
		SELECT
			count(*) FILTER (WHERE deleted IS NULL),
			count(*) FILTER (WHERE status = 'revoked')
		FROM tenant.organizations_users
		WHERE organization_id = '019b4000-0000-7000-8000-000000000002'`).Scan(&liveMemberships, &revokedMemberships)
	require.NoError(t, err)
	assert.Equal(t, 0, liveMemberships)
	assert.Positive(t, revokedMemberships)

	err = ctx.DB.Pool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM authn.api_keys
		WHERE organization_id = '019b4000-0000-7000-8000-000000000002'
		  AND revoked IS NULL`).Scan(&liveKeys)
	require.NoError(t, err)
	assert.Equal(t, 0, liveKeys)
}

func TestOrganizationDelete_DeletesOrganizationWhoseMembersWereRemoved(t *testing.T) {
	ctx := newTestContext(t)

	require.NoError(t, (&OrganizationCreateCmd{Name: "once-staffed"}).Run(ctx))
	require.NoError(t, (&OrganizationMemberAddCmd{Organization: "once-staffed", User: "alice@acme-corp.com", Permission: "viewer"}).Run(ctx))
	require.NoError(t, (&OrganizationMemberRemoveCmd{Organization: "once-staffed", User: "alice@acme-corp.com"}).Run(ctx))

	require.NoError(t, (&OrganizationDeleteCmd{Name: "once-staffed"}).Run(ctx))

	live, _ := organizationRows(t, ctx, "once-staffed")
	assert.Equal(t, 0, live)
}

func TestOrganizationDelete_RefusesOrganizationWithClusters(t *testing.T) {
	ctx := newTestContext(t)

	err := (&OrganizationDeleteCmd{Name: "acme-corp"}).Run(ctx)
	require.EqualError(t, err, "organization 'acme-corp' still has 1 cluster(s); delete them first and wait until they are torn down")

	live, _ := organizationRows(t, ctx, "acme-corp")
	assert.Equal(t, 1, live)
	members, err := ctx.Queries.MembershipList(t.Context(), membershipListParamsFor(t, ctx, "acme-corp"))
	require.NoError(t, err)
	assert.Len(t, members, 3, "nothing is revoked when the delete is refused")
}

func TestOrganizationDelete_WaitsForClusterTeardown(t *testing.T) {
	ctx := newTestContext(t)

	// Globex has no clusters. Give it one that is deleted but whose shoot
	// Gardener has not confirmed gone yet.
	var clusterID string
	err := ctx.DB.Pool.QueryRow(t.Context(), `
		INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version, deleted, shoot_status)
		VALUES ('019b4000-0000-7000-8000-000000000002', 'tearing-down', 'local', '1.31', now(), 'deleting')
		RETURNING id`).Scan(&clusterID)
	require.NoError(t, err)

	err = (&OrganizationDeleteCmd{Name: "globex"}).Run(ctx)
	require.EqualError(t, err, "organization 'globex' still has 1 cluster(s); delete them first and wait until they are torn down")

	_, err = ctx.DB.Pool.Exec(t.Context(), `UPDATE tenant.clusters SET shoot_status = 'deleted' WHERE id = $1`, clusterID)
	require.NoError(t, err)

	require.NoError(t, (&OrganizationDeleteCmd{Name: "globex"}).Run(ctx))
}

func TestOrganizationCreate_RefusesNameOfDeletedOrganizationWithClusters(t *testing.T) {
	ctx := newTestContext(t)

	// Globex has no clusters. Give it one Gardener has finished tearing down:
	// the Gardener project named after globex is still there.
	_, err := ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version, deleted, shoot_status)
		VALUES ('019b4000-0000-7000-8000-000000000002', 'torn-down', 'local', '1.31', now(), 'deleted')`)
	require.NoError(t, err)

	require.NoError(t, (&OrganizationDeleteCmd{Name: "globex"}).Run(ctx))

	err = (&OrganizationCreateCmd{Name: "globex"}).Run(ctx)
	require.EqualError(t, err, "organization name 'globex' is still held by a deleted organization that had clusters; its Gardener project is not cleaned up yet")

	_, all := organizationRows(t, ctx, "globex")
	assert.Equal(t, 1, all, "no new organization is created")
}

func TestOrganizationDelete_RefusesOrganizationWithPlugins(t *testing.T) {
	ctx := newTestContext(t)

	_, err := ctx.DB.Pool.Exec(t.Context(), `
		INSERT INTO appstore.plugins (organization_id, name, description)
		VALUES ('019b4000-0000-7000-8000-000000000002', 'globex-plugin', 'A plugin globex publishes.')`)
	require.NoError(t, err)

	err = (&OrganizationDeleteCmd{Name: "globex"}).Run(ctx)
	require.EqualError(t, err, "organization 'globex' still publishes 1 plugin(s); delete them first")
}

func TestOrganizationDelete_AlreadyDeletedIsNotFound(t *testing.T) {
	ctx := newTestContext(t)

	require.NoError(t, (&OrganizationCreateCmd{Name: "gone"}).Run(ctx))
	require.NoError(t, (&OrganizationDeleteCmd{Name: "gone"}).Run(ctx))

	err := (&OrganizationDeleteCmd{Name: "gone"}).Run(ctx)
	require.EqualError(t, err, "organization 'gone' not found")

	_, all := organizationRows(t, ctx, "gone")
	assert.Equal(t, 1, all, "a second delete does not remove the soft-deleted row")
}
