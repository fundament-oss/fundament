-- Region catalog fixtures for local development (TREK_INSERT_TEST_DATA only).
--
-- The console's create-cluster wizard is catalog-driven (ListRegions): without
-- catalog rows there is nothing to choose. This seeds a single `local` region
-- backed by Gardener's local provider. Real environments do NOT get seeded:
-- their catalog is operator-managed data inserted after deployment.
--
-- The local provider ignores the worker machine type (the cluster-worker
-- substitutes its DefaultMachineType), so the machine types below only shape
-- the dev UI.

INSERT INTO catalog.regions (id, name, cloud_profile, cloud_profile_region) VALUES
    ('019b4000-5000-7000-8000-000000000001', 'local', 'local', 'local');

INSERT INTO catalog.kubernetes_versions (id, version) VALUES
    ('019b4000-5100-7000-8000-000000000001', '1.34.0'),
    ('019b4000-5100-7000-8000-000000000002', '1.33.0');

INSERT INTO catalog.machine_types (id, name, lcpu, memory) VALUES
    ('019b4000-5200-7000-8000-000000000001', 'local-small', 2, 4294967296),   -- 4 GiB
    ('019b4000-5200-7000-8000-000000000002', 'local-medium', 4, 8589934592);  -- 8 GiB

INSERT INTO catalog.region_kubernetes_versions (region_id, kubernetes_version_id) VALUES
    ('019b4000-5000-7000-8000-000000000001', '019b4000-5100-7000-8000-000000000001'),
    ('019b4000-5000-7000-8000-000000000001', '019b4000-5100-7000-8000-000000000002');

INSERT INTO catalog.region_machine_types (id, region_id, machine_type_id) VALUES
    ('019b4000-5300-7000-8000-000000000001', '019b4000-5000-7000-8000-000000000001', '019b4000-5200-7000-8000-000000000001'),
    ('019b4000-5300-7000-8000-000000000002', '019b4000-5000-7000-8000-000000000001', '019b4000-5200-7000-8000-000000000002');

-- ── eu-west-1 (test fixtures region) ────────────────────────────────────────
-- The tenant fixtures (001) and the terraform-provider acceptance tests create
-- clusters in `eu-west-1` with versions 1.28/1.29/1.31.0 and n1-standard
-- machine types; creation is catalog-validated, so those offerings must exist.
INSERT INTO catalog.regions (id, name, cloud_profile, cloud_profile_region) VALUES
    ('019b4000-5000-7000-8000-000000000002', 'eu-west-1', 'local', 'local');

INSERT INTO catalog.kubernetes_versions (id, version) VALUES
    ('019b4000-5100-7000-8000-000000000003', '1.28'),
    ('019b4000-5100-7000-8000-000000000004', '1.29'),
    ('019b4000-5100-7000-8000-000000000005', '1.31.0');

INSERT INTO catalog.machine_types (id, name, lcpu, memory) VALUES
    ('019b4000-5200-7000-8000-000000000003', 'n1-standard-1', 1, 4026531840),  -- 3.75 GiB
    ('019b4000-5200-7000-8000-000000000004', 'n1-standard-2', 2, 8053063680);  -- 7.5 GiB

INSERT INTO catalog.region_kubernetes_versions (region_id, kubernetes_version_id) VALUES
    ('019b4000-5000-7000-8000-000000000002', '019b4000-5100-7000-8000-000000000003'),
    ('019b4000-5000-7000-8000-000000000002', '019b4000-5100-7000-8000-000000000004'),
    ('019b4000-5000-7000-8000-000000000002', '019b4000-5100-7000-8000-000000000005');

INSERT INTO catalog.region_machine_types (id, region_id, machine_type_id) VALUES
    ('019b4000-5300-7000-8000-000000000003', '019b4000-5000-7000-8000-000000000002', '019b4000-5200-7000-8000-000000000003'),
    ('019b4000-5300-7000-8000-000000000004', '019b4000-5000-7000-8000-000000000002', '019b4000-5200-7000-8000-000000000004');

-- Link the pre-catalog tenant fixtures to the catalog (the region-match trigger
-- rejects node pools on clusters whose region_id is NULL).
UPDATE tenant.clusters SET
    region_id = catalog.regions.id,
    kubernetes_version_id = catalog.kubernetes_versions.id
FROM catalog.regions, catalog.kubernetes_versions
WHERE tenant.clusters.region = catalog.regions.name
  AND tenant.clusters.kubernetes_version = catalog.kubernetes_versions.version
  AND tenant.clusters.region_id IS NULL;
