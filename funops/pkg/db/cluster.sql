-- name: ClusterQueueSync :many
-- Queues a sync of one cluster, or of every cluster when no id is given: the
-- cluster-worker then writes the cluster's Shoot again, as it does after a
-- change to the cluster. Deleted clusters are left out; their Shoots are on
-- the way out. A cluster that already has a sync waiting is left out too.
INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
SELECT tenant.clusters.id, 'updated', 'manual'
FROM tenant.clusters
WHERE tenant.clusters.deleted IS NULL
  AND (sqlc.narg('cluster_id')::uuid IS NULL OR tenant.clusters.id = sqlc.narg('cluster_id')::uuid)
  AND NOT EXISTS (
    SELECT 1
    FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
      AND tenant.cluster_outbox.event = 'updated'
      AND tenant.cluster_outbox.status = 'pending'
  )
RETURNING cluster_id;
