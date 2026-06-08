-- name: LogicalDesignList :many
SELECT id, name, version, description, status, created
FROM dcim.logical_designs
WHERE deleted IS NULL
ORDER BY created;

-- name: LogicalDesignGetByID :one
SELECT id, name, version, description, status, created
FROM dcim.logical_designs
WHERE id = $1 AND deleted IS NULL;

-- name: LogicalDesignCreate :one
INSERT INTO dcim.logical_designs (name, description)
VALUES ($1, $2)
RETURNING id;

-- name: LogicalDesignUpdate :execrows
UPDATE dcim.logical_designs
SET name        = COALESCE(sqlc.narg('name'), name),
    description = COALESCE(sqlc.narg('description'), description),
    status      = COALESCE(sqlc.narg('status'), status)
WHERE id = $1 AND deleted IS NULL;

-- name: LogicalDesignDelete :execrows
UPDATE dcim.logical_designs
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
