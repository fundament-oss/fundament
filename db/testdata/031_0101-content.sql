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
