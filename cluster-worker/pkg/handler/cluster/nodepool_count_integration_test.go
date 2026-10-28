package cluster_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/dbconst"
)

// A pool's node counts are stored whole or not at all: a NULL in the
// populated branch must not let the CHECK pass as UNKNOWN.
func TestNodePoolCountsAreAllOrNothing(t *testing.T) {
	t.Parallel()
	db := createTestDB(t)

	clusterID := insertCluster(t, db, acmeCorpOrgID, "tf-counts")
	poolID := insertNodePoolReturningID(t, db, clusterID, "workers", "n1-standard-4", 1, 3)

	_, err := db.adminPool.Exec(t.Context(),
		`UPDATE tenant.node_pools SET node_count = 2, node_count_ready = 1, node_count_updated = now() WHERE id = $1`, poolID)
	require.NoError(t, err)

	for name, set := range map[string]string{
		"no count":              `node_count = NULL, node_count_ready = 0, node_count_updated = now()`,
		"no ready count":        `node_count = 2, node_count_ready = NULL, node_count_updated = now()`,
		"only a time":           `node_count = NULL, node_count_ready = NULL, node_count_updated = now()`,
		"no time":               `node_count = 2, node_count_ready = 1, node_count_updated = NULL`,
		"more ready than nodes": `node_count = 1, node_count_ready = 2, node_count_updated = now()`,
	} {
		_, err := db.adminPool.Exec(t.Context(), `UPDATE tenant.node_pools SET `+set+` WHERE id = $1`, poolID)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr, name)
		assert.Equal(t, dbconst.ConstraintNodePoolsCkNodeCount, pgErr.ConstraintName, name)
	}

	_, err = db.adminPool.Exec(t.Context(),
		`UPDATE tenant.node_pools SET node_count = NULL, node_count_ready = NULL, node_count_updated = NULL WHERE id = $1`, poolID)
	require.NoError(t, err)
}
