-- System organization — owns first-party (seeded) plugins.
--
-- Applied in ALL environments (incl. production) by the db-migrations Job before
-- the appstore catalog seed, so appstore.plugins.organization_id FK resolves.
-- Idempotent upsert keyed on the stable UUID below; has no members, so normal
-- users never see it while the plugins it owns stay globally visible.

INSERT INTO tenant.organizations (id, name, alias) VALUES
    ('019b4000-0000-7000-8000-000000000000', 'system', 'System')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, alias = EXCLUDED.alias;
