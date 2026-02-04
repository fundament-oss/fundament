-- Backfill outbox events for users that were seeded before the outbox triggers existed
INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
SELECT
    'user',
    u.id::text,
    'created',
    jsonb_build_object(
        'user_id', u.id,
        'organization_id', u.organization_id,
        'role', u.role
    )
FROM tenant.users u
WHERE u.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.aggregate_type = 'user'
    AND o.aggregate_id = u.id::text
    AND o.event_type = 'created'
);
