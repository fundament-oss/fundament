-- Backfill outbox events for users that were seeded before the outbox triggers existed
INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
SELECT
    'users',
    u.id::text,
    'created',
    to_jsonb(u)
FROM tenant.users u
WHERE u.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.aggregate_type = 'users'
    AND o.aggregate_id = u.id::text
    AND o.event_type = 'created'
);
