-- name: DeviceCatalogList :many
SELECT id, manufacturer, model, part_number, category, form_factor, rack_units, weight_kg, power_draw_w, specs, created
FROM dcim.device_catalogs
WHERE deleted IS NULL
  AND (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category')::text)
  AND (sqlc.narg('search')::text IS NULL OR manufacturer ILIKE '%' || sqlc.narg('search')::text || '%' OR model ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY manufacturer, model;

-- name: DeviceCatalogGetByID :one
SELECT id, manufacturer, model, part_number, category, form_factor, rack_units, weight_kg, power_draw_w, specs, created
FROM dcim.device_catalogs
WHERE id = $1 AND deleted IS NULL;

-- name: DeviceCatalogCreate :one
INSERT INTO dcim.device_catalogs (manufacturer, model, part_number, category, form_factor, rack_units, weight_kg, power_draw_w, specs)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: DeviceCatalogUpdate :execrows
UPDATE dcim.device_catalogs
SET manufacturer = COALESCE(sqlc.narg('manufacturer'), manufacturer),
    model        = COALESCE(sqlc.narg('model'), model),
    part_number  = COALESCE(sqlc.narg('part_number'), part_number),
    category     = COALESCE(sqlc.narg('category'), category),
    form_factor  = COALESCE(sqlc.narg('form_factor'), form_factor),
    rack_units   = COALESCE(sqlc.narg('rack_units'), rack_units),
    weight_kg    = COALESCE(sqlc.narg('weight_kg'), weight_kg),
    power_draw_w = COALESCE(sqlc.narg('power_draw_w'), power_draw_w),
    specs        = COALESCE(sqlc.narg('specs'), specs)
WHERE id = $1 AND deleted IS NULL;

-- name: DeviceCatalogDelete :execrows
UPDATE dcim.device_catalogs
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;

-- name: DeviceCatalogAssetCounts :many
SELECT device_catalog_id,
       count(*)::int AS total,
       count(*) FILTER (WHERE status = 'deployed')::int AS deployed,
       count(*) FILTER (WHERE status = 'in_stock')::int AS available,
       count(*) FILTER (WHERE status = 'rma')::int AS needs_repair
FROM dcim.assets
WHERE deleted IS NULL
  AND device_catalog_id = ANY(sqlc.arg('ids')::uuid[])
GROUP BY device_catalog_id;
