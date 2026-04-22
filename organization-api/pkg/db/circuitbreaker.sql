-- name: AuthzOutboxLagSeconds :one
-- Returns the age in seconds of the oldest unprocessed outbox entry,
-- or 0 when no pending/retrying rows exist.
SELECT COALESCE(EXTRACT(EPOCH FROM (now() - MIN(authz.outbox.created))), 0)::float8
FROM authz.outbox
WHERE authz.outbox.processed IS NULL
  AND authz.outbox.status IN ('pending', 'retrying');
