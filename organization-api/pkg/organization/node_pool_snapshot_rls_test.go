package organization_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The API reads node pool snapshots of its own organization's clusters only.
func Test_NodePoolSnapshots_RLS_Policies(t *testing.T) {
	t.Parallel()

	ownerOrgID := uuid.New()
	otherOrgID := uuid.New()
	ownerID := uuid.New()
	otherID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(ownerOrgID, "owner-org"),
		WithOrganization(otherOrgID, "other-org"),
		WithUser(&UserArgs{ID: ownerID, Name: "owner-user", OrgIDs: []uuid.UUID{ownerOrgID}}),
		WithUser(&UserArgs{ID: otherID, Name: "other-user", OrgIDs: []uuid.UUID{otherOrgID}}),
	)

	ownerCluster, _, _ := nodePoolTestCluster(t, env, ownerOrgID, env.createAuthnToken(t, ownerID))
	otherCluster, _, _ := nodePoolTestCluster(t, env, otherOrgID, env.createAuthnToken(t, otherID))
	for _, clusterID := range []string{ownerCluster, otherCluster} {
		_, err := env.adminPool.Exec(t.Context(),
			`INSERT INTO tenant.node_pool_snapshots (node_pool_id, observed, nodes, nodes_ready)
			 SELECT tenant.node_pools.id, now(), 1, 1 FROM tenant.node_pools WHERE tenant.node_pools.cluster_id = $1`, clusterID)
		require.NoError(t, err)
	}

	// Connect as the RLS-subject role (fun_fundament_api) to this test's DB.
	dbName := testNameToDbName(t.Name())
	conn, err := pgx.Connect(t.Context(),
		fmt.Sprintf("postgres://fun_fundament_api@localhost:%d/%s?sslmode=disable", testDBPort, dbName))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	_, err = conn.Exec(t.Context(), "SELECT set_config('app.current_organization_id', $1, false)", ownerOrgID.String())
	require.NoError(t, err)

	var ownerRows, otherRows int
	countFor := `SELECT count(*) FROM tenant.node_pool_snapshots
		JOIN tenant.node_pools ON tenant.node_pools.id = tenant.node_pool_snapshots.node_pool_id
		WHERE tenant.node_pools.cluster_id = $1`
	require.NoError(t, conn.QueryRow(t.Context(), countFor, ownerCluster).Scan(&ownerRows))
	require.NoError(t, conn.QueryRow(t.Context(), countFor, otherCluster).Scan(&otherRows))
	assert.Equal(t, 2, ownerRows, "the organization's own pools")
	assert.Zero(t, otherRows, "another organization's pools")

	// The snapshot alone, without the join, is just as hidden.
	var all int
	require.NoError(t, conn.QueryRow(t.Context(), "SELECT count(*) FROM tenant.node_pool_snapshots").Scan(&all))
	assert.Equal(t, 2, all)
}
