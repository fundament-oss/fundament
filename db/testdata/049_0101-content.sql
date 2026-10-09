-- Quotas for the tenant fixtures (TREK_INSERT_TEST_DATA only).
--
-- A new organization starts with quota_clusters = 1 and no machine quotas, so
-- nothing can be built in it until an operator sets quotas with
-- `funops organization quota`. The fixture organizations get room to spare: the
-- integration tests create clusters and node pools in them freely, and so does
-- the local development stack. Real environments are not seeded; their quotas
-- are set per organization by the operator.

UPDATE tenant.organizations
SET quota_clusters = 100
WHERE name IN ('acme-corp', 'globex', 'initech');

INSERT INTO tenant.organization_machine_quotas (organization_id, region_machine_type_id, max_nodes)
SELECT tenant.organizations.id, catalog.region_machine_types.id, 100
FROM tenant.organizations
CROSS JOIN catalog.region_machine_types
WHERE tenant.organizations.name IN ('acme-corp', 'globex', 'initech');
