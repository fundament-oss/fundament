-- name: TaskStepList :many
SELECT id, task_id, title, description, ordinal, completed, created
FROM dcim.task_steps
WHERE task_id = $1 AND deleted IS NULL
ORDER BY ordinal;

-- name: TaskStepCreate :one
INSERT INTO dcim.task_steps (task_id, title, description, ordinal)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: TaskStepUpdate :execrows
UPDATE dcim.task_steps
SET title       = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    ordinal     = COALESCE(sqlc.narg('ordinal'), ordinal),
    completed   = COALESCE(sqlc.narg('completed'), completed)
WHERE id = $1 AND deleted IS NULL;

-- name: TaskStepDelete :execrows
UPDATE dcim.task_steps
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
