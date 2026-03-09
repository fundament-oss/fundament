-- Grant schema_migrations read access to API roles (needed for dbversion.MustAssertLatestVersion)
GRANT SELECT ON public.schema_migrations TO fun_fundament_api, fun_authn_api;

-- Project for acme-corp and its members in one transaction (deferred trigger requires admin before commit)
BEGIN;

INSERT INTO tenant.projects (id, cluster_id, name) VALUES
    ('019b4000-9000-7000-8000-000000000001', '019b4000-2000-7000-8000-000000000001', 'acme-project');

INSERT INTO tenant.project_members (id, project_id, user_id, role) VALUES
    ('019b4000-a000-7000-8000-000000000001', '019b4000-9000-7000-8000-000000000001', '019b4000-1000-7000-8000-000000000002', 'admin'),
    ('019b4000-a000-7000-8000-000000000002', '019b4000-9000-7000-8000-000000000001', '019b4000-1000-7000-8000-000000000003', 'viewer');

COMMIT;
