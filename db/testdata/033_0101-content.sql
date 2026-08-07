-- Platform maintainer: admin of the system org so a login can publish the
-- first-party plugins' definitions. At 033 because the system org is created there.

INSERT INTO tenant.users (id, name, email, external_ref) VALUES
    ('019b4000-1000-7000-8000-000000000008', 'Platform Admin', 'platform-admin@fundament.io', 'CiQwMTliNDAwMC0xMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDgSBWxvY2Fs');

INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status) VALUES
    ('019b4000-0000-7000-8000-000000000000', '019b4000-1000-7000-8000-000000000008', 'admin', 'accepted');
