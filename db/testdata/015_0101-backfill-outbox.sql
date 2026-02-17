-- Backfill outbox events for records that were seeded before the outbox triggers existed
UPDATE tenant.users
SET role='admin'
WHERE id='019b4000-1000-7000-8000-000000000001';

UPDATE tenant.users
SET role='admin'
WHERE id='019b4000-1000-7000-8000-000000000004';

UPDATE tenant.users
SET role='admin'
WHERE id='019b4000-1000-7000-8000-000000000006';

INSERT INTO authz.outbox (user_id)
SELECT tenant.users.id
FROM tenant.users
WHERE tenant.users.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox
    WHERE authz.outbox.user_id = tenant.users.id
);

INSERT INTO authz.outbox (project_id)
SELECT tenant.projects.id
FROM tenant.projects
WHERE tenant.projects.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox
    WHERE authz.outbox.project_id = tenant.projects.id
);

INSERT INTO authz.outbox (project_member_id)
SELECT tenant.project_members.id
FROM tenant.project_members
WHERE tenant.project_members.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox
    WHERE authz.outbox.project_member_id = tenant.project_members.id
);

INSERT INTO authz.outbox (cluster_id)
SELECT tenant.clusters.id
FROM tenant.clusters
WHERE tenant.clusters.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox
    WHERE authz.outbox.cluster_id = tenant.clusters.id
);

INSERT INTO authz.outbox (node_pool_id)
SELECT tenant.node_pools.id
FROM tenant.node_pools
WHERE tenant.node_pools.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox
    WHERE authz.outbox.node_pool_id = tenant.node_pools.id
);

INSERT INTO authz.outbox (namespace_id)
SELECT tenant.namespaces.id
FROM tenant.namespaces
WHERE tenant.namespaces.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox
    WHERE authz.outbox.namespace_id = tenant.namespaces.id
);

INSERT INTO authz.outbox (api_key_id)
SELECT authn.api_keys.id
FROM authn.api_keys
WHERE authn.api_keys.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox
    WHERE authz.outbox.api_key_id = authn.api_keys.id
);

INSERT INTO authz.outbox (install_id)
SELECT appstore.installs.id
FROM appstore.installs
WHERE appstore.installs.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox
    WHERE authz.outbox.install_id = appstore.installs.id
);
