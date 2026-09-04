-- name: TaskList :many
-- Tags come along as an array so a list of tasks stays one round trip; a task
-- without tags gets an empty array rather than a row full of NULLs.
SELECT t.id, t.title, t.description, t.status, t.priority, t.blocked_reason, t.assignee_id, t.due_date, t.location, t.created,
       COALESCE(tags.list, ARRAY[]::text[])::text[] AS tags
FROM dcim.tasks t
LEFT JOIN LATERAL (
  SELECT array_agg(tt.tag ORDER BY tt.tag) AS list
  FROM dcim.task_tags tt
  WHERE tt.task_id = t.id
) tags ON TRUE
WHERE t.deleted IS NULL
  AND (sqlc.narg('status')::text IS NULL OR t.status = sqlc.narg('status')::text)
  AND (sqlc.narg('priority')::text IS NULL OR t.priority = sqlc.narg('priority')::text)
  AND (sqlc.narg('tag')::text IS NULL OR EXISTS (
        SELECT 1 FROM dcim.task_tags f WHERE f.task_id = t.id AND f.tag = sqlc.narg('tag')::text))
  AND (sqlc.narg('assignee_id')::uuid IS NULL OR t.assignee_id = sqlc.narg('assignee_id')::uuid)
ORDER BY t.created DESC;

-- name: TaskGetByID :one
SELECT t.id, t.title, t.description, t.status, t.priority, t.blocked_reason, t.assignee_id, t.due_date, t.location, t.created,
       COALESCE(tags.list, ARRAY[]::text[])::text[] AS tags
FROM dcim.tasks t
LEFT JOIN LATERAL (
  SELECT array_agg(tt.tag ORDER BY tt.tag) AS list
  FROM dcim.task_tags tt
  WHERE tt.task_id = t.id
) tags ON TRUE
WHERE t.id = $1 AND t.deleted IS NULL;

-- name: TaskCreate :one
INSERT INTO dcim.tasks (title, description, status, priority, blocked_reason, assignee_id, due_date, location)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: TaskUpdate :execrows
UPDATE dcim.tasks
SET title       = COALESCE(sqlc.narg('title'), title),
    description = CASE
                    WHEN sqlc.arg('clear_description')::bool THEN NULL
                    ELSE COALESCE(sqlc.narg('description'), description)
                  END,
    status      = COALESCE(sqlc.narg('status'), status),
    priority    = COALESCE(sqlc.narg('priority'), priority),
    blocked_reason = CASE
                    WHEN sqlc.arg('clear_blocked_reason')::bool THEN NULL
                    ELSE COALESCE(sqlc.narg('blocked_reason'), blocked_reason)
                  END,
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

-- name: TaskTagsClear :exec
DELETE FROM dcim.task_tags WHERE task_id = $1;

-- name: TaskTagsAdd :exec
-- One statement for the whole set, and the same tag twice is not an error: the
-- caller sends what the task should carry, not what changed.
INSERT INTO dcim.task_tags (task_id, tag)
SELECT $1, unnest(@tags::text[])
ON CONFLICT (task_id, tag) DO NOTHING;
