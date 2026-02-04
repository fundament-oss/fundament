-- Backfill outbox events for records that were seeded before the outbox triggers existed

INSERT INTO authz.outbox (user_id)
SELECT u.id
FROM tenant.users u
WHERE u.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.user_id = u.id
);

INSERT INTO authz.outbox (project_id)
SELECT p.id
FROM tenant.projects p
WHERE p.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.project_id = p.id
);

INSERT INTO authz.outbox (project_member_id)
SELECT pm.id
FROM tenant.project_members pm
WHERE pm.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.project_member_id = pm.id
);

INSERT INTO authz.outbox (cluster_id)
SELECT c.id
FROM tenant.clusters c
WHERE c.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.cluster_id = c.id
);

INSERT INTO authz.outbox (node_pool_id)
SELECT np.id
FROM tenant.node_pools np
WHERE np.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.node_pool_id = np.id
);

INSERT INTO authz.outbox (namespace_id)
SELECT n.id
FROM tenant.namespaces n
WHERE n.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.namespace_id = n.id
);

INSERT INTO authz.outbox (api_key_id)
SELECT ak.id
FROM authn.api_keys ak
WHERE ak.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.api_key_id = ak.id
);

INSERT INTO authz.outbox (install_id)
SELECT i.id
FROM zappstore.installs i
WHERE i.deleted IS NULL
AND NOT EXISTS (
    SELECT 1 FROM authz.outbox o
    WHERE o.install_id = i.id
);
