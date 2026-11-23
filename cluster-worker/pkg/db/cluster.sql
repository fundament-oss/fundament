-- name: ClusterCreateSyncSucceededEvent :one
-- Insert sync_succeeded event when Gardener accepts the manifest.
INSERT INTO
    tenant.cluster_events (
        cluster_id,
        event_type,
        sync_action,
        message
    )
VALUES
    (
        @cluster_id,
        'sync_succeeded',
        @sync_action,
        @message
    )
RETURNING
    id;

-- name: ClusterCreateSyncFailedEvent :one
-- Insert sync_failed event for history.
INSERT INTO
    tenant.cluster_events (
        cluster_id,
        event_type,
        sync_action,
        message,
        attempt
    )
VALUES
    (
        @cluster_id,
        'sync_failed',
        @sync_action,
        @message,
        @attempt
    )
RETURNING
    id;

-- name: ClusterListActive :many
-- Used by periodic reconciliation to compare with Gardener state.
-- latest_sync_completed: the cluster's own latest sync row completed, so a
-- missing Shoot is not one a queued or failing sync has yet to create. "Latest
-- sync row" is the cluster_sync_state view's row
-- (TestClusterListActiveAgreesWithClusterSyncState pins the two together).
-- The version and update flag feed the drift findings reconcile records.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_updating,
    tenant.clusters.kubernetes_version,
    tenant.organizations.name AS organization_name,
    COALESCE((
        SELECT tenant.cluster_outbox.status
        FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
          AND tenant.cluster_outbox.event IN ('created', 'updated', 'deleted', 'reconcile')
        ORDER BY tenant.cluster_outbox.id DESC
        LIMIT 1
    ) = 'completed', false)::boolean AS latest_sync_completed
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.deleted IS NULL
ORDER BY
    tenant.clusters.id;

-- name: ClusterListDeletedByIDs :many
-- Of the given cluster IDs, those whose cluster row exists and is soft-deleted.
-- Periodic reconciliation only deletes a Shoot whose cluster a user deleted,
-- and only once the delete sync has settled.
-- sync_running: the cluster's own latest sync row (as in ClusterGetPushState)
-- is still queued.
SELECT
    tenant.clusters.id,
    COALESCE((
        SELECT tenant.cluster_outbox.status IN ('pending', 'retrying')
        FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
          AND tenant.cluster_outbox.event IN ('created', 'updated', 'deleted', 'reconcile')
        ORDER BY tenant.cluster_outbox.id DESC
        LIMIT 1
    ), false)::boolean AS sync_running
FROM
    tenant.clusters
WHERE
    tenant.clusters.id = ANY(@cluster_ids::uuid[])
    AND tenant.clusters.deleted IS NOT NULL
ORDER BY
    tenant.clusters.id;

-- name: NodePoolListByClusterID :many
-- Fetch active (non-deleted) node pools for a cluster.
-- Used by the cluster handler to build Gardener worker groups.
-- The catalog join resolves the machine-type name when the pool references
-- region_machine_types; legacy pools fall back to the machine_type text column.
SELECT
    tenant.node_pools.id,
    tenant.node_pools.name,
    COALESCE(catalog.machine_types.name, tenant.node_pools.machine_type) AS machine_type,
    tenant.node_pools.autoscale_min,
    tenant.node_pools.autoscale_max,
    tenant.node_pools.created
FROM
    tenant.node_pools
    LEFT JOIN catalog.region_machine_types ON catalog.region_machine_types.id = tenant.node_pools.region_machine_type_id
    LEFT JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
WHERE
    tenant.node_pools.cluster_id = @cluster_id
    AND tenant.node_pools.deleted IS NULL
ORDER BY
    tenant.node_pools.created,
    tenant.node_pools.id;

-- name: NodePoolListStatusByClusterID :many
-- The status path needs only identity, the machine type its reason names, the
-- minimum its own nodes are held to and when it was created (older nodes are a
-- predecessor's), the stored classification, and the node counts last
-- observed (tenant.node_pool_snapshots, shared with the inventory worker); the
-- worker-group build query above, with its catalog joins, stays with sync.
-- status_updated is the value the status write must still find: a check that
-- decided from an older report must not land after a newer one.
-- outbox_superseded says a sync completed after the pool's latest row: a sync
-- applies the whole cluster, so the pool's failed row no longer describes what
-- Gardener has, whichever pool or the cluster itself the later row belonged to.
SELECT
    tenant.node_pools.id,
    tenant.node_pools.name,
    tenant.node_pools.machine_type,
    tenant.node_pools.autoscale_min,
    tenant.node_pools.created,
    tenant.node_pools.status,
    tenant.node_pools.status_message,
    tenant.node_pools.status_updated,
    tenant.node_pool_snapshots.nodes,
    tenant.node_pool_snapshots.nodes_ready,
    tenant.node_pool_snapshots.observed,
    COALESCE((
        SELECT tenant.cluster_outbox.status
        FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.node_pool_id = tenant.node_pools.id
        ORDER BY tenant.cluster_outbox.id DESC
        LIMIT 1
    ), '')::text AS outbox_status,
    (
        SELECT tenant.cluster_outbox.status_info
        FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.node_pool_id = tenant.node_pools.id
        ORDER BY tenant.cluster_outbox.id DESC
        LIMIT 1
    ) AS outbox_status_info,
    EXISTS (
        SELECT 1
        FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.status = 'completed'
          AND (
              (
                  tenant.cluster_outbox.cluster_id = @cluster_id::uuid
                  AND tenant.cluster_outbox.event IN ('created', 'updated', 'deleted', 'reconcile')
              )
              OR EXISTS (
                  SELECT 1
                  FROM tenant.node_pools
                  WHERE tenant.node_pools.id = tenant.cluster_outbox.node_pool_id
                    AND tenant.node_pools.cluster_id = @cluster_id::uuid
              )
          )
          -- A pool's sync only applies the cluster once it reached Gardener:
          -- before that it queues the cluster's own sync and completes having
          -- changed nothing, so it does not supersede a failed pool row.
          AND EXISTS (
              SELECT 1
              FROM tenant.cluster_outbox
              WHERE tenant.cluster_outbox.cluster_id = @cluster_id::uuid
                AND tenant.cluster_outbox.status = 'completed'
          )
          -- Ids order the inserts, not the runs: the worker claims by
          -- retry_after, so a row in backoff runs after higher ids. One that
          -- finished after the pool's row failed applied the cluster, and that
          -- pool's change, all the same.
          AND (
              tenant.cluster_outbox.id > (
                  SELECT tenant.cluster_outbox.id
                  FROM tenant.cluster_outbox
                  WHERE tenant.cluster_outbox.node_pool_id = tenant.node_pools.id
                  ORDER BY tenant.cluster_outbox.id DESC
                  LIMIT 1
              )
              OR tenant.cluster_outbox.processed > (
                  SELECT tenant.cluster_outbox.failed
                  FROM tenant.cluster_outbox
                  WHERE tenant.cluster_outbox.node_pool_id = tenant.node_pools.id
                  ORDER BY tenant.cluster_outbox.id DESC
                  LIMIT 1
              )
          )
    )::boolean AS outbox_superseded
FROM
    tenant.node_pools
    LEFT JOIN tenant.node_pool_snapshots ON tenant.node_pool_snapshots.node_pool_id = tenant.node_pools.id
WHERE
    tenant.node_pools.cluster_id = @cluster_id
    AND tenant.node_pools.deleted IS NULL
ORDER BY
    tenant.node_pools.created,
    tenant.node_pools.id;

-- name: ClusterGetPushState :one
-- What the outbox still owes Gardener, read AFTER the pool list: a pool's
-- INSERT commits together with its cluster_outbox row, so any pool the list
-- returned has its outbox row visible here. The state read with the cluster row
-- at the start of a check is stale for pools inserted since.
--
-- sync_in_flight: a change was saved but its sync has not applied it to
-- Gardener — the cluster's own latest sync row is queued or retrying, or an
-- active node pool's latest row (its create or update) is. A deleted pool's
-- row is not counted: removing a pool does not shape the pools that remain.
-- sync_queued: some sync of the cluster is still to come, whatever it is: any
-- of the cluster's or its pools' rows is queued or retrying, a deleted pool's
-- included, since such a sync creates or updates the whole Shoot. The status
-- timeout waits for one rather than record a Shoot that never appeared just
-- before the sync that creates it.
-- sync_blocked: the cluster's own latest sync row failed for good, and no
-- pool sync came after it that can still apply it; nothing will retry it, so
-- the pools must say so instead of waiting behind it. Any pool row counts here,
-- a deleted pool's included, since such a sync applies the whole cluster to
-- Gardener, failed change and all. Two later rows do not lift it: one that
-- failed itself, and one of a cluster that never reached Gardener — neither
-- sync applied the cluster's change.
WITH latest_cluster AS (
    SELECT
        tenant.cluster_outbox.id,
        tenant.cluster_outbox.status,
        tenant.cluster_outbox.status_info,
        tenant.cluster_outbox.failed
    FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.cluster_id = @cluster_id
      AND tenant.cluster_outbox.event IN ('created', 'updated', 'deleted', 'reconcile')
    ORDER BY tenant.cluster_outbox.id DESC
    LIMIT 1
),
-- The pool rows that came after the cluster's own latest sync. Ids order the
-- inserts, not the runs: the worker claims by retry_after, so a row in backoff
-- runs after higher ids. A queued row counts by its id, a completed one also by
-- having finished after the failure was recorded, when its handler read the
-- change that the failed row never applied.
superseding_pool_row AS (
    SELECT 1
    FROM tenant.cluster_outbox
    JOIN tenant.node_pools ON tenant.node_pools.id = tenant.cluster_outbox.node_pool_id
    CROSS JOIN latest_cluster
    WHERE tenant.node_pools.cluster_id = @cluster_id
      AND tenant.cluster_outbox.status <> 'failed'
      -- A pool's sync only applies the cluster once it reached Gardener: before
      -- that it queues the cluster's own sync and completes having changed
      -- nothing, so it cannot supersede a failed cluster row. The same
      -- "reached Gardener" test as ClusterHasEverBeenSynced, which gates that
      -- early return.
      AND EXISTS (
          SELECT 1
          FROM tenant.cluster_outbox
          WHERE tenant.cluster_outbox.cluster_id = @cluster_id
            AND tenant.cluster_outbox.status = 'completed'
      )
      AND (
          tenant.cluster_outbox.id > latest_cluster.id
          OR tenant.cluster_outbox.processed > latest_cluster.failed
      )
)
SELECT
    tenant.clusters.shoot_updating,
    (
        COALESCE((SELECT latest_cluster.status IN ('pending', 'retrying') FROM latest_cluster), false)
        OR EXISTS (
            SELECT 1
            FROM tenant.node_pools
            WHERE tenant.node_pools.cluster_id = tenant.clusters.id
              AND tenant.node_pools.deleted IS NULL
              AND COALESCE((
                  SELECT tenant.cluster_outbox.status IN ('pending', 'retrying')
                  FROM tenant.cluster_outbox
                  WHERE tenant.cluster_outbox.node_pool_id = tenant.node_pools.id
                  ORDER BY tenant.cluster_outbox.id DESC
                  LIMIT 1
              ), false)
        )
    )::boolean AS sync_in_flight,
    (
        EXISTS (
            SELECT 1
            FROM tenant.cluster_outbox
            WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
              AND tenant.cluster_outbox.event IN ('created', 'updated', 'deleted', 'reconcile')
              AND tenant.cluster_outbox.status IN ('pending', 'retrying')
        )
        OR EXISTS (
            SELECT 1
            FROM tenant.cluster_outbox
            JOIN tenant.node_pools ON tenant.node_pools.id = tenant.cluster_outbox.node_pool_id
            WHERE tenant.node_pools.cluster_id = tenant.clusters.id
              AND tenant.cluster_outbox.status IN ('pending', 'retrying')
        )
    )::boolean AS sync_queued,
    COALESCE((
        SELECT latest_cluster.status = 'failed'
            AND NOT EXISTS (SELECT 1 FROM superseding_pool_row)
        FROM latest_cluster
    ), false)::boolean AS sync_blocked,
    (
        SELECT latest_cluster.status_info
        FROM latest_cluster
        WHERE latest_cluster.status = 'failed'
          AND NOT EXISTS (SELECT 1 FROM superseding_pool_row)
    ) AS sync_blocked_message
FROM
    tenant.clusters
WHERE
    tenant.clusters.id = @cluster_id;

-- name: NodePoolUpdateStatus :execrows
-- Record what Gardener reports about one pool's machines. Keyed by id: the
-- row was just read by the same check, and with soft deletes (cluster_id,
-- name) could meanwhile mean a recreated pool. Writing only while
-- status_updated still holds the value read with the row keeps a check that
-- classified from an older report from landing after a newer one; zero rows
-- says so. A cluster another check recorded as lost since this one read it
-- had its pools cleared, and that clear must stand. @cluster_status is the
-- status this check read, so a check that read the lost itself still writes:
-- its pools are classified from the live report that brought the Shoot back.
UPDATE tenant.node_pools
SET
    status = @status,
    status_message = @message,
    status_updated = now()
WHERE
    tenant.node_pools.id = @node_pool_id
    AND tenant.node_pools.deleted IS NULL
    AND tenant.node_pools.status_updated IS NOT DISTINCT FROM @checked_at
    AND (
        @cluster_status::text = 'lost'
        OR EXISTS (
            SELECT 1
            FROM tenant.clusters
            WHERE tenant.clusters.id = tenant.node_pools.cluster_id
              AND tenant.clusters.shoot_status IS DISTINCT FROM 'lost'
        )
    );

-- name: NodePoolClearStatusByClusterID :exec
-- Forget what was recorded about a lost cluster's pools: their machines went
-- with the Shoot. When a change creates it again they read as new, so as
-- progressing while the create runs, instead of their old status and reason.
UPDATE tenant.node_pools
SET
    status = NULL,
    status_message = NULL,
    status_updated = now()
WHERE
    cluster_id = @cluster_id
    AND deleted IS NULL;

-- name: NodePoolGetClusterID :one
-- Returns the cluster_id for a node pool (including soft-deleted node pools).
-- Used by the cluster handler to resolve node_pool_id → cluster_id.
SELECT
    tenant.node_pools.cluster_id
FROM
    tenant.node_pools
WHERE
    tenant.node_pools.id = @node_pool_id;

-- name: ClusterHasEverBeenSynced :one
-- Returns whether a cluster has been successfully synced to Gardener at least once.
-- Checks outbox history directly (EXISTS on completed rows with cluster_id set).
-- This remains true even if the latest outbox row is retrying or failed.
-- The same "completed outbox row" test is spelled in ClusterListForStatusSweep,
-- ClusterGetForStatusCheck (checkable) and the reached_gardener columns of
-- organization-api's cluster queries and platform-api/pkg/db/fleet.sql; change
-- them together.
SELECT EXISTS (
    SELECT 1
    FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.cluster_id = @cluster_id
      AND tenant.cluster_outbox.status = 'completed'
)::boolean AS has_been_synced;

-- name: ClusterListForStatusSweep :many
-- IDs of every cluster the status sweep checks. A deleted cluster is always
-- checked: one that never reached Gardener still has to be recorded as
-- deleted, or it keeps its name and its quota. An active cluster is checked
-- once a check can learn something: a status was recorded, an outbox row
-- completed, or a sync gave up. A create that failed for good has no Shoot to
-- wait for, and its status and its pools still have to say why.
SELECT
    tenant.clusters.id
FROM
    tenant.clusters
WHERE
    (
        tenant.clusters.deleted IS NOT NULL
        OR tenant.clusters.shoot_status IS NOT NULL
        -- reached Gardener: the test ClusterHasEverBeenSynced spells out
        OR EXISTS (
            SELECT 1
            FROM tenant.cluster_outbox
            WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
              AND tenant.cluster_outbox.status = 'completed'
        )
        OR EXISTS (
            SELECT 1
            FROM tenant.cluster_outbox
            WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
              AND tenant.cluster_outbox.status = 'failed'
        )
    )
    AND (
        tenant.clusters.deleted IS NULL
        OR tenant.clusters.shoot_status IS DISTINCT FROM 'deleted'
    )
ORDER BY
    tenant.clusters.id;

-- name: ClusterGetForStatusCheck :one
-- Load one cluster for a status check, active or soft-deleted. checkable says
-- whether a check can learn anything: the cluster reached Gardener (a status
-- was recorded or an outbox row completed) or a sync of it gave up. Before
-- either there is no Shoot to read and no failure to record.
-- shoot_status_since is when the stored status began, so a check knows how
-- long one has lasted; update_settled is when the cluster's latest sync
-- finished, so a check knows how long Gardener has had to roll out a change.
-- A node pool's sync applies the whole cluster too, and it carries no
-- cluster_id, so its rows count here as well. It is NULL while that row is
-- still queued: the previous row's finish is no measure of an update Gardener
-- has not received yet. A failed row is NULL too: it never delivered the
-- change, so its failure time is no measure either.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_health,
    tenant.clusters.shoot_updating,
    tenant.clusters.shoot_api_available,
    tenant.clusters.organization_id,
    tenant.clusters.shoot_status_updated,
    -- A row from before the column existed takes its last status write as the
    -- start of the status: otherwise a cluster already stuck at deploy time
    -- would never reach a timeout, since an unchanged status writes nothing.
    COALESCE(tenant.clusters.shoot_status_since, tenant.clusters.shoot_status_updated) AS shoot_status_since,
    tenant.organizations.name AS organization_name,
    catalog.regions.cloud_profile,
    catalog.regions.cloud_profile_region,
    (
        SELECT tenant.cluster_outbox.processed
        FROM tenant.cluster_outbox
        LEFT JOIN tenant.node_pools ON tenant.node_pools.id = tenant.cluster_outbox.node_pool_id
        WHERE (
            tenant.cluster_outbox.cluster_id = tenant.clusters.id
            AND tenant.cluster_outbox.event IN ('created', 'updated', 'deleted', 'reconcile')
        )
        OR tenant.node_pools.cluster_id = tenant.clusters.id
        ORDER BY tenant.cluster_outbox.id DESC
        LIMIT 1
    ) AS update_settled,
    (
        tenant.clusters.shoot_status IS NOT NULL
        -- reached Gardener: the test ClusterHasEverBeenSynced spells out
        OR EXISTS (
            SELECT 1
            FROM tenant.cluster_outbox
            WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
              AND tenant.cluster_outbox.status = 'completed'
        )
        -- A sync that gave up: no Shoot may exist, but its status and its
        -- pools still have to say why.
        OR EXISTS (
            SELECT 1
            FROM tenant.cluster_outbox
            WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
              AND tenant.cluster_outbox.status = 'failed'
        )
    )::boolean AS checkable
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN catalog.regions ON catalog.regions.id = tenant.clusters.region_id
WHERE
    tenant.clusters.id = @cluster_id;

-- name: ClusterUpdateShootStatus :execrows
-- Update shoot status from Gardener polling. health is NULL unless ready;
-- updating is set while an update fundament pushed rolls out. Writes nothing
-- when the row changed since it was read (checked_at is the
-- shoot_status_updated read then), such as the sync handler marking an update.
-- shoot_status_since moves only when the status itself changes, so timeouts
-- measure how long a status lasted, not how long since its message last moved;
-- a row from before the column was added takes the first moment it is seen.
-- shoot_api_available keeps the first time the API server was seen available;
-- api_gone clears it when the Shoot is gone or being deleted, so a recreated
-- Shoot is announced again.
UPDATE tenant.clusters
SET
    shoot_status = @status,
    shoot_status_message = @message,
    shoot_health = @health,
    shoot_updating = @updating,
    shoot_api_available = CASE
        WHEN @api_gone::boolean THEN NULL
        WHEN @api_available::boolean THEN COALESCE(shoot_api_available, now())
        ELSE shoot_api_available
    END,
    shoot_status_since = CASE
        WHEN shoot_status IS DISTINCT FROM @status THEN now()
        ELSE COALESCE(shoot_status_since, now())
    END,
    shoot_status_updated = now()
WHERE
    id = @cluster_id
    AND shoot_status_updated IS NOT DISTINCT FROM @checked_at;

-- name: ClusterMarkShootUpdating :execrows
-- Mark a ready cluster as updating once Gardener accepted a spec change. It
-- stays ready, so what is gated on a ready cluster (kubeconfig, member sync)
-- keeps working while the update rolls out. A cluster still settling after
-- its create keeps the awaiting-health message: organization-api keys the
-- STARTING status on that exact text, and an update mid-settle must not
-- relabel the cluster unhealthy.
UPDATE tenant.clusters
SET
    shoot_updating = true,
    shoot_status_message = CASE
        WHEN tenant.clusters.shoot_status_message = @awaiting_message THEN tenant.clusters.shoot_status_message
        ELSE @message
    END,
    shoot_status_updated = now()
WHERE
    id = @cluster_id
    AND shoot_status = 'ready';

-- name: ClusterListWarningMessages :many
-- The messages of the cluster's status_warning events since its last milestone
-- (ready, healthy, error or lost), so an error Gardener retries again and again
-- is recorded once, and the same error in a later incident is recorded again.
-- All of them, not the latest: two warning sources can be active at once, and
-- compared with one message each would re-add the other's warning every check.
-- The cutoff is inclusive: created is the transaction timestamp, so a warning
-- written by the same status update as a milestone shares its created, and a
-- strict comparison would drop it and record it again on every check. A
-- milestone is never a status_warning, so the tie costs nothing.
SELECT DISTINCT
    tenant.cluster_events.message
FROM
    tenant.cluster_events
WHERE
    tenant.cluster_events.cluster_id = @cluster_id
    AND tenant.cluster_events.event_type = 'status_warning'
    AND tenant.cluster_events.message IS NOT NULL
    AND tenant.cluster_events.created >= COALESCE((
        SELECT
            max(tenant.cluster_events.created)
        FROM
            tenant.cluster_events
        WHERE
            tenant.cluster_events.cluster_id = @cluster_id
            AND tenant.cluster_events.event_type IN ('status_ready', 'status_healthy', 'status_error', 'status_lost')
    ), '-infinity'::timestamptz);

-- name: ClusterGetLastProgressEvent :one
-- The cluster's most recent status_progressing event, which throttles how
-- often Gardener's progress messages are recorded.
SELECT
    tenant.cluster_events.created,
    tenant.cluster_events.message
FROM
    tenant.cluster_events
WHERE
    tenant.cluster_events.cluster_id = @cluster_id
    AND tenant.cluster_events.event_type = 'status_progressing'
ORDER BY
    tenant.cluster_events.created DESC
LIMIT
    1;

-- name: ClusterMarkShootDeleted :execrows
-- Record that a deleted cluster has no Shoot (it never reached Gardener, or
-- the Shoot is already gone), so it stops counting against the organization's
-- cluster quota at once instead of at the next status sweep. The same end
-- state the sweep writes when it confirms a deletion.
UPDATE tenant.clusters
SET
    shoot_status = 'deleted',
    shoot_status_message = @message,
    shoot_health = NULL,
    shoot_updating = false,
    shoot_api_available = NULL,
    shoot_status_since = now(),
    shoot_status_updated = now()
WHERE
    id = @cluster_id
    AND deleted IS NOT NULL
    AND shoot_status IS DISTINCT FROM 'deleted';

-- name: ClusterCreateEventUnlessLatest :execrows
-- Insert an event unless the cluster's latest one already says the same (type
-- and message): a cluster waiting on a precondition records the wait once,
-- however many of its outbox rows park meanwhile. The caller holds the
-- cluster row (OutboxLockCluster), so two parks cannot both insert.
INSERT INTO tenant.cluster_events (cluster_id, event_type, message)
SELECT @cluster_id, @event_type, @message
WHERE (
    SELECT tenant.cluster_events.event_type = @event_type::text
       AND tenant.cluster_events.message IS NOT DISTINCT FROM @message::text
    FROM tenant.cluster_events
    WHERE tenant.cluster_events.cluster_id = @cluster_id
    ORDER BY tenant.cluster_events.created DESC, tenant.cluster_events.id DESC
    LIMIT 1
) IS DISTINCT FROM true;

-- name: ClusterCreateStatusEvent :one
-- Insert status event (only for milestone states: ready, error, deleted).
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, message)
VALUES
    (@cluster_id, @event_type, @message)
RETURNING
    id;
