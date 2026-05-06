-- name: SiteList :many
SELECT id, name, address, created
FROM dcim.sites
WHERE deleted IS NULL
ORDER BY created;

-- name: SiteGetByID :one
SELECT id, name, address, created
FROM dcim.sites
WHERE id = $1 AND deleted IS NULL;

-- name: SiteCreate :one
INSERT INTO dcim.sites (name, address)
VALUES ($1, $2)
RETURNING id;

-- name: SiteUpdate :execrows
UPDATE dcim.sites
SET name    = COALESCE(sqlc.narg('name'), name),
    address = COALESCE(sqlc.narg('address'), address)
WHERE id = $1 AND deleted IS NULL;

-- name: SiteDelete :execrows
UPDATE dcim.sites
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
