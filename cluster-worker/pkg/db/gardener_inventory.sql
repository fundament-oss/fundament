-- name: GardenerShootUpsert :exec
-- Record the shoots one ListShoots returned, the arrays zipped by position.
-- A shoot seen before keeps its first_seen; last_seen moves to this run.
INSERT INTO
    tenant.gardener_shoots (
        shoot_uid,
        shoot_name,
        shoot_namespace,
        cluster_id,
        kubernetes_version,
        region,
        shoot_created,
        first_seen,
        last_seen
    )
SELECT
    unnest(@shoot_uids::text[]),
    unnest(@shoot_names::text[]),
    unnest(@shoot_namespaces::text[]),
    unnest(@cluster_ids::uuid[]),
    unnest(@kubernetes_versions::text[]),
    unnest(@regions::text[]),
    unnest(@shoot_created::timestamptz[]),
    @seen::timestamptz,
    @seen::timestamptz
ON CONFLICT ON CONSTRAINT gardener_shoots_uq_shoot_uid DO UPDATE
SET
    shoot_name = EXCLUDED.shoot_name,
    shoot_namespace = EXCLUDED.shoot_namespace,
    cluster_id = EXCLUDED.cluster_id,
    kubernetes_version = EXCLUDED.kubernetes_version,
    region = EXCLUDED.region,
    shoot_created = EXCLUDED.shoot_created,
    last_seen = EXCLUDED.last_seen;

-- name: GardenerShootMarkGone :execrows
-- Soft-delete the shoots the same ListShoots no longer returned. Only called
-- after a successful list: a failed one says nothing about what is gone.
UPDATE tenant.gardener_shoots
SET
    deleted = @seen
WHERE
    tenant.gardener_shoots.deleted IS NULL
    AND NOT (tenant.gardener_shoots.shoot_uid = ANY (@shoot_uids::text[]));
