-- name: TaskList :many
SELECT id, title, description, status, priority, category, assignee_id, due_date, location, created
FROM dcim.tasks
WHERE deleted IS NULL
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('priority')::text IS NULL OR priority = sqlc.narg('priority')::text)
  AND (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category')::text)
  AND (sqlc.narg('assignee_id')::uuid IS NULL OR assignee_id = sqlc.narg('assignee_id')::uuid)
ORDER BY created DESC;

-- name: TaskGetByID :one
SELECT id, title, description, status, priority, category, assignee_id, due_date, location, created
FROM dcim.tasks
WHERE id = $1 AND deleted IS NULL;

-- name: TaskCreate :one
INSERT INTO dcim.tasks (title, description, status, priority, category, assignee_id, due_date, location)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: TaskUpdate :execrows
UPDATE dcim.tasks
SET title       = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    status      = COALESCE(sqlc.narg('status'), status),
    priority    = COALESCE(sqlc.narg('priority'), priority),
    category    = COALESCE(sqlc.narg('category'), category),
    assignee_id = CASE
                    WHEN sqlc.arg('clear_assignee')::bool THEN NULL
                    ELSE COALESCE(sqlc.narg('assignee_id'), assignee_id)
                  END,
    due_date    = CASE
                    WHEN sqlc.arg('clear_due_date')::bool THEN NULL
                    ELSE COALESCE(sqlc.narg('due_date'), due_date)
                  END,
    location    = CASE
                    WHEN sqlc.arg('clear_location')::bool THEN NULL
                    ELSE COALESCE(sqlc.narg('location'), location)
                  END
WHERE id = $1 AND deleted IS NULL;

-- name: TaskDelete :execrows
UPDATE dcim.tasks
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
