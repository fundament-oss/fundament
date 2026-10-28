-- name: DriftFindingList :many
-- Open drift findings as cluster-worker's reconcile loop recorded them, with
-- the cluster and organization they concern where those exist. Findings on
-- the system organization's clusters are left out, as everywhere. Paged by
-- (kind, first_detected, id) after the given key.
SELECT
    tenant.drift_findings.id,
    tenant.drift_findings.kind,
    tenant.drift_findings.cluster_id,
    tenant.drift_findings.shoot_name,
    tenant.drift_findings.shoot_namespace,
    tenant.drift_findings.expected,
    tenant.drift_findings.observed,
    tenant.drift_findings.first_detected,
    tenant.drift_findings.last_detected,
    tenant.clusters.name AS cluster_name,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name
FROM
    tenant.drift_findings
    LEFT JOIN tenant.clusters ON tenant.clusters.id = tenant.drift_findings.cluster_id
    LEFT JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.drift_findings.resolved IS NULL
    AND tenant.clusters.organization_id IS DISTINCT FROM tenant.system_organization_id()
    AND (
        sqlc.narg('after_id')::uuid IS NULL
        OR (tenant.drift_findings.kind, tenant.drift_findings.first_detected, tenant.drift_findings.id)
            > (sqlc.narg('after_kind')::text, sqlc.narg('after_first_detected')::timestamptz, sqlc.narg('after_id')::uuid)
    )
ORDER BY
    tenant.drift_findings.kind,
    tenant.drift_findings.first_detected,
    tenant.drift_findings.id
LIMIT @page_limit::integer;
