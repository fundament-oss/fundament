-- name: AssetList :many
SELECT id, device_catalog_id, serial_number, asset_tag, purchase_date, purchase_order, warranty_expiry, status, notes, created
FROM dcim.assets
WHERE (sqlc.narg('include_deleted')::bool IS NOT TRUE AND deleted IS NULL OR sqlc.narg('include_deleted')::bool IS TRUE)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('device_catalog_id')::uuid IS NULL OR device_catalog_id = sqlc.narg('device_catalog_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL OR serial_number ILIKE '%' || sqlc.narg('search')::text || '%' OR asset_tag ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY created;

-- name: AssetGetByID :one
SELECT id, device_catalog_id, serial_number, asset_tag, purchase_date, purchase_order, warranty_expiry, status, notes, created
FROM dcim.assets
WHERE id = $1 AND deleted IS NULL;

-- name: AssetCreate :one
INSERT INTO dcim.assets (device_catalog_id, serial_number, asset_tag, purchase_date, purchase_order, warranty_expiry, status, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: AssetUpdate :execrows
UPDATE dcim.assets
SET status          = COALESCE(sqlc.narg('status'), status),
    serial_number   = COALESCE(sqlc.narg('serial_number'), serial_number),
    asset_tag       = COALESCE(sqlc.narg('asset_tag'), asset_tag),
    warranty_expiry = COALESCE(sqlc.narg('warranty_expiry'), warranty_expiry),
    notes           = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1 AND deleted IS NULL;

-- name: AssetDelete :execrows
UPDATE dcim.assets
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;

-- name: AssetListByCatalogID :many
SELECT id, device_catalog_id, serial_number, asset_tag, purchase_date, purchase_order, warranty_expiry, status, notes, created
FROM dcim.assets
WHERE device_catalog_id = $1 AND deleted IS NULL
ORDER BY created;

-- name: AssetEventList :many
SELECT id, asset_id, event_type, details, performed_by, created
FROM dcim.asset_events
WHERE asset_id = $1
ORDER BY created;

-- name: AssetEventCreate :one
INSERT INTO dcim.asset_events (asset_id, event_type, details, performed_by)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: AssetStats :one
SELECT
    count(*)::int AS total,
    count(*) FILTER (WHERE status = 'in_stock')::int AS available,
    count(*) FILTER (WHERE status = 'deployed')::int AS deployed,
    count(*) FILTER (WHERE status = 'rma')::int AS needs_repair,
    count(*) FILTER (WHERE status = 'in_transit')::int AS on_order,
    count(*) FILTER (WHERE status = 'reserved')::int AS requested,
    count(*) FILTER (WHERE status = 'decommissioned')::int AS decommissioned
FROM dcim.assets
WHERE deleted IS NULL;
