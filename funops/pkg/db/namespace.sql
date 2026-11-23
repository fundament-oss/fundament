-- name: NamespaceQueueSync :many
-- Queues a reconcile of one namespace, or of every namespace whose latest sync
-- failed when no id is given: the cluster-worker then converges the namespace
-- again, as it does after a change to the namespace. A namespace that already
-- has a row waiting is left out; a second run changes nothing. A deleted
-- namespace is included: its row still has to remove what the namespace left
-- on the shoot.
-- The --all branch leaves out namespaces of a cluster that is deleted or lost
-- its Shoot: no sync of them can converge, so queueing them only adds failures
-- to triage. A Shoot that is created again syncs them through its api_available
-- fan-out. A namespace named by id is queued whatever its cluster's state.
-- The NOT EXISTS guard is not backed by ON CONFLICT here: the partial unique
-- index cluster_outbox_uq_ns_reconcile covers source 'reconcile' only, and
-- widening it takes a migration. Two operators queueing one namespace at once
-- can both pass the guard; that costs one extra sync of an idempotent handler,
-- never a wrong result.
INSERT INTO tenant.cluster_outbox (namespace_id, event, source)
SELECT tenant.namespaces.id, 'reconcile', 'manual'
FROM tenant.namespaces
WHERE (
    (
        sqlc.narg('namespace_id')::uuid IS NOT NULL
        AND tenant.namespaces.id = sqlc.narg('namespace_id')::uuid
    ) OR (
        sqlc.narg('namespace_id')::uuid IS NULL
        AND COALESCE((
            SELECT tenant.cluster_outbox.status = 'failed'
            FROM tenant.cluster_outbox
            WHERE tenant.cluster_outbox.namespace_id = tenant.namespaces.id
            ORDER BY tenant.cluster_outbox.id DESC
            LIMIT 1
        ), false)
        AND EXISTS (
            SELECT 1
            FROM tenant.projects
            JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
            WHERE tenant.projects.id = tenant.namespaces.project_id
              AND tenant.clusters.deleted IS NULL
              AND tenant.clusters.shoot_status IS DISTINCT FROM 'lost'
        )
    )
)
AND NOT EXISTS (
    SELECT 1
    FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.namespace_id = tenant.namespaces.id
      AND tenant.cluster_outbox.status IN ('pending', 'retrying')
)
RETURNING namespace_id;
