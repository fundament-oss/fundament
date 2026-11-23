-- name: NamespaceQueueSync :many
-- Queues a reconcile of one namespace, or of every namespace whose latest sync
-- failed when no id is given: the cluster-worker then converges the namespace
-- again, as it does after a change to the namespace. A namespace that already
-- has a row waiting is left out; a second run changes nothing. A deleted
-- namespace is included: its row still has to remove what the namespace left
-- on the shoot.
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
    )
)
AND NOT EXISTS (
    SELECT 1
    FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.namespace_id = tenant.namespaces.id
      AND tenant.cluster_outbox.status IN ('pending', 'retrying')
)
RETURNING namespace_id;
