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

-- name: DriftFindingUpsert :exec
-- Record what one reconcile pass found, the arrays zipped by position. A
-- finding still open keeps its first_detected; last_detected moves to this
-- pass. Empty strings stand for NULL, as pgx cannot send NULL array elements
-- for uuid.UUID.
INSERT INTO
    tenant.drift_findings (
        kind,
        cluster_id,
        shoot_uid,
        shoot_name,
        shoot_namespace,
        expected,
        observed,
        first_detected,
        last_detected
    )
SELECT
    unnest(@kinds::text[]),
    nullif(unnest(@cluster_ids::text[]), '')::uuid,
    nullif(unnest(@shoot_uids::text[]), ''),
    nullif(unnest(@shoot_names::text[]), ''),
    nullif(unnest(@shoot_namespaces::text[]), ''),
    nullif(unnest(@expected::text[]), ''),
    nullif(unnest(@observed::text[]), ''),
    @seen::timestamptz,
    @seen::timestamptz
ON CONFLICT ON CONSTRAINT drift_findings_uq_subject DO UPDATE
SET
    shoot_name = EXCLUDED.shoot_name,
    shoot_namespace = EXCLUDED.shoot_namespace,
    expected = EXCLUDED.expected,
    observed = EXCLUDED.observed,
    last_detected = EXCLUDED.last_detected;

-- name: DriftFindingResolve :execrows
-- Resolve every open finding the pass at seen did not detect again.
UPDATE tenant.drift_findings
SET
    resolved = @seen
WHERE
    tenant.drift_findings.resolved IS NULL
    AND tenant.drift_findings.last_detected < @seen;

-- name: WorkerHeartbeatRecord :exec
-- Record that a periodic loop ran a pass, and how often it runs. Reconcile
-- records a finished pass; inventory one whose clusters are queued.
INSERT INTO
    tenant.worker_heartbeats (
        worker,
        interval_seconds,
        last_pass
    )
VALUES
    (
        @worker,
        @interval_seconds,
        @last_pass
    )
ON CONFLICT ON CONSTRAINT worker_heartbeats_uq_worker DO UPDATE
SET
    interval_seconds = EXCLUDED.interval_seconds,
    last_pass = EXCLUDED.last_pass;
