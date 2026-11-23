package cli

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pendingManualNamespaceSyncs lists the namespaces with a queued reconcile
// from funops.
func pendingManualNamespaceSyncs(t *testing.T, ctx *Context) []uuid.UUID {
	t.Helper()

	rows, err := ctx.DB.Pool.Query(t.Context(), `
		SELECT namespace_id
		FROM tenant.cluster_outbox
		WHERE source = 'manual' AND event = 'reconcile' AND status = 'pending'`)
	require.NoError(t, err)
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

// namespaceTestFixtures gives globex a live cluster with a project and the
// namespaces named, and settles the outbox rows the inserts queued, so only
// rows from the command under test are pending.
func namespaceTestFixtures(t *testing.T, ctx *Context, names ...string) (namespaces []uuid.UUID) {
	t.Helper()

	const globex = "019b4000-0000-7000-8000-000000000002"
	var clusterID uuid.UUID
	err := ctx.DB.Pool.QueryRow(t.Context(), `
		INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version)
		VALUES ($1, 'namespace-sync', 'local', '1.31')
		RETURNING id`, globex).Scan(&clusterID)
	require.NoError(t, err)

	var projectID uuid.UUID
	err = ctx.DB.Pool.QueryRow(t.Context(), `
		WITH p AS (
		    INSERT INTO tenant.projects (cluster_id, name, alias)
		    VALUES ($1, 'namespace-sync', 'namespace-sync') RETURNING id
		), u AS (
		    INSERT INTO tenant.users (name) VALUES ('namespace-sync-admin') RETURNING id
		), m AS (
		    INSERT INTO tenant.project_members (project_id, user_id, role)
		    SELECT p.id, u.id, 'admin' FROM p, u
		)
		SELECT id FROM p`, clusterID).Scan(&projectID)
	require.NoError(t, err)

	for _, name := range names {
		var id uuid.UUID
		err := ctx.DB.Pool.QueryRow(t.Context(), `
			INSERT INTO tenant.namespaces (project_id, name) VALUES ($1, $2)
			RETURNING id`, projectID, name).Scan(&id)
		require.NoError(t, err)
		namespaces = append(namespaces, id)
	}

	_, err = ctx.DB.Pool.Exec(t.Context(), `UPDATE tenant.cluster_outbox SET status = 'completed', processed = now()`)
	require.NoError(t, err)
	return namespaces
}

// failNamespaceSync marks every outbox row of a namespace failed for good.
func failNamespaceSync(t *testing.T, ctx *Context, namespaceID uuid.UUID) {
	t.Helper()

	_, err := ctx.DB.Pool.Exec(t.Context(), `
		UPDATE tenant.cluster_outbox SET status = 'failed', failed = now(), status_info = 'cannot succeed'
		WHERE namespace_id = $1`, namespaceID)
	require.NoError(t, err)
}

// One namespace ID queues a reconcile of that namespace only.
func TestNamespaceSync_Arguments(t *testing.T) {
	ctx := newTestContext(t)

	require.EqualError(t, (&NamespaceSyncCmd{}).Run(ctx), "give a namespace ID or --all")
	require.EqualError(t, (&NamespaceSyncCmd{Namespace: uuid.NewString(), All: true}).Run(ctx),
		"give a namespace ID or --all, not both")
	require.EqualError(t, (&NamespaceSyncCmd{Namespace: "nope"}).Run(ctx), `namespace ID "nope" is not a UUID`)
}

func TestNamespaceSync_One(t *testing.T) {
	ctx := newTestContext(t)
	namespaces := namespaceTestFixtures(t, ctx, "alpha", "beta")

	require.NoError(t, (&NamespaceSyncCmd{Namespace: namespaces[0].String()}).Run(ctx))
	assert.Equal(t, []uuid.UUID{namespaces[0]}, pendingManualNamespaceSyncs(t, ctx))

	// A second run queues nothing and says so: the namespace already has a
	// sync waiting, so the operator does not take it for a queued sync.
	err := (&NamespaceSyncCmd{Namespace: namespaces[0].String()}).Run(ctx)
	require.Error(t, err)
	assert.Len(t, pendingManualNamespaceSyncs(t, ctx), 1)
}

// --all queues the namespaces whose last sync failed, and only those.
func TestNamespaceSync_All(t *testing.T) {
	ctx := newTestContext(t)
	namespaces := namespaceTestFixtures(t, ctx, "failed", "completed")
	failNamespaceSync(t, ctx, namespaces[0])

	require.NoError(t, (&NamespaceSyncCmd{All: true}).Run(ctx))
	assert.Equal(t, []uuid.UUID{namespaces[0]}, pendingManualNamespaceSyncs(t, ctx))

	require.NoError(t, (&NamespaceSyncCmd{All: true}).Run(ctx))
	assert.Len(t, pendingManualNamespaceSyncs(t, ctx), 1, "a namespace with a sync waiting is not queued again")
}

// A deleted namespace that failed is re-driven too: its sync still has to
// remove what it left on the shoot.
func TestNamespaceSync_All_IncludesDeletedNamespace(t *testing.T) {
	ctx := newTestContext(t)
	namespaces := namespaceTestFixtures(t, ctx, "gone")
	_, err := ctx.DB.Pool.Exec(t.Context(),
		`UPDATE tenant.namespaces SET deleted = now() WHERE id = $1`, namespaces[0])
	require.NoError(t, err)
	failNamespaceSync(t, ctx, namespaces[0])

	require.NoError(t, (&NamespaceSyncCmd{All: true}).Run(ctx))
	assert.Equal(t, []uuid.UUID{namespaces[0]}, pendingManualNamespaceSyncs(t, ctx))
}
