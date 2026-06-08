-- name: LogicalDeviceLayoutGetByDesign :many
SELECT logical_device_layouts.id, logical_device_layouts.logical_device_id, logical_device_layouts.position_x, logical_device_layouts.position_y, logical_device_layouts.updated
FROM dcim.logical_device_layouts
JOIN dcim.logical_devices ON logical_devices.id = logical_device_layouts.logical_device_id
WHERE logical_devices.logical_design_id = $1
  AND logical_devices.deleted IS NULL;

-- name: LogicalDeviceLayoutUpsert :one
INSERT INTO dcim.logical_device_layouts (logical_device_id, position_x, position_y)
VALUES ($1, $2, $3)
ON CONFLICT (logical_device_id) DO UPDATE
SET position_x = EXCLUDED.position_x,
    position_y = EXCLUDED.position_y,
    updated    = now()
RETURNING id, logical_device_id, position_x, position_y, updated;

-- name: LogicalDeviceLayoutDeleteByDesign :exec
DELETE FROM dcim.logical_device_layouts
WHERE logical_device_id IN (
    SELECT id FROM dcim.logical_devices WHERE logical_design_id = $1
);

-- name: LogicalDeviceLayoutDeleteNotIn :exec
DELETE FROM dcim.logical_device_layouts
WHERE logical_device_id IN (
    SELECT id FROM dcim.logical_devices WHERE logical_design_id = sqlc.arg('logical_design_id')::uuid
)
  AND logical_device_id <> ALL(sqlc.arg('keep')::uuid[]);
