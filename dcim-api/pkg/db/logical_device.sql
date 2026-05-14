-- name: LogicalDeviceList :many
SELECT id, logical_design_id, label, role, device_catalog_id, requirements, notes, created
FROM dcim.logical_devices
WHERE logical_design_id = $1 AND deleted IS NULL
ORDER BY created;

-- name: LogicalDeviceGetByID :one
SELECT id, logical_design_id, label, role, device_catalog_id, requirements, notes, created
FROM dcim.logical_devices
WHERE id = $1 AND deleted IS NULL;

-- name: LogicalDeviceCreate :one
INSERT INTO dcim.logical_devices (logical_design_id, label, role, device_catalog_id, requirements, notes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: LogicalDeviceUpdate :execrows
UPDATE dcim.logical_devices
SET label             = COALESCE(sqlc.narg('label'), label),
    role              = COALESCE(sqlc.narg('role'), role),
    device_catalog_id = COALESCE(sqlc.narg('device_catalog_id'), device_catalog_id),
    requirements      = COALESCE(sqlc.narg('requirements'), requirements),
    notes             = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1 AND deleted IS NULL;

-- name: LogicalDeviceDelete :execrows
UPDATE dcim.logical_devices
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
