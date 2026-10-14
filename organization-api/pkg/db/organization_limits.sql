-- name: OrganizationLimitsGet :one
SELECT
    default_memory_request_mi,
    default_memory_limit_mi,
    default_cpu_request_m,
    default_cpu_limit_m
FROM tenant.organization_limits
WHERE organization_id = @organization_id
  AND deleted IS NULL;

-- name: OrganizationLimitsUpsert :one
INSERT INTO tenant.organization_limits (
    organization_id,
    default_memory_request_mi,
    default_memory_limit_mi,
    default_cpu_request_m,
    default_cpu_limit_m
) VALUES (
    @organization_id,
    @default_memory_request_mi,
    @default_memory_limit_mi,
    @default_cpu_request_m,
    @default_cpu_limit_m
)
ON CONFLICT ON CONSTRAINT organization_limits_uq_org DO UPDATE SET
    default_memory_request_mi  = EXCLUDED.default_memory_request_mi,
    default_memory_limit_mi    = EXCLUDED.default_memory_limit_mi,
    default_cpu_request_m      = EXCLUDED.default_cpu_request_m,
    default_cpu_limit_m        = EXCLUDED.default_cpu_limit_m
RETURNING
    default_memory_request_mi,
    default_memory_limit_mi,
    default_cpu_request_m,
    default_cpu_limit_m;
