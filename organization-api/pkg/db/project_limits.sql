-- name: ProjectLimitsGet :one
SELECT
    default_memory_request_mi,
    default_memory_limit_mi,
    default_cpu_request_m,
    default_cpu_limit_m
FROM tenant.project_limits
WHERE project_id = @project_id
  AND deleted IS NULL;

-- name: ProjectLimitsUpsert :one
INSERT INTO tenant.project_limits (
    project_id,
    default_memory_request_mi,
    default_memory_limit_mi,
    default_cpu_request_m,
    default_cpu_limit_m
) VALUES (
    @project_id,
    @default_memory_request_mi,
    @default_memory_limit_mi,
    @default_cpu_request_m,
    @default_cpu_limit_m
)
ON CONFLICT ON CONSTRAINT project_limits_uq_project DO UPDATE SET
    default_memory_request_mi = EXCLUDED.default_memory_request_mi,
    default_memory_limit_mi   = EXCLUDED.default_memory_limit_mi,
    default_cpu_request_m     = EXCLUDED.default_cpu_request_m,
    default_cpu_limit_m       = EXCLUDED.default_cpu_limit_m
RETURNING
    default_memory_request_mi,
    default_memory_limit_mi,
    default_cpu_request_m,
    default_cpu_limit_m;
