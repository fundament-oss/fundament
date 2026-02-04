-- Backfill outbox events for users that were seeded before the outbox triggers existed
INSERT INTO authz.outbox (user_id)
SELECT u.id
FROM tenant.users u
WHERE u.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.user_id = u.id
);
