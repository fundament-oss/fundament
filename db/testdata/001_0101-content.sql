-- Test data for local development
-- Note: The 'admin' user and organization are created by the authn-api on first login

-- Additional organizations
INSERT INTO tenant.organizations (id, name) VALUES
    ('019b4000-0000-7000-8000-000000000001', 'acme-corp'),
    ('019b4000-0000-7000-8000-000000000002', 'globex'),
    ('019b4000-0000-7000-8000-000000000003', 'initech');

-- Users for each organization
INSERT INTO tenant.users (id, organization_id, name, external_id, role, email) VALUES
    -- acme-corp users
    ('019b4000-1000-7000-8000-000000000001', '019b4000-0000-7000-8000-000000000001', 'Alice', 'CiQwMTliNDAwMC0xMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDESBWxvY2Fs', 'admin', 'alice@acme-corp.com'),
    ('019b4000-1000-7000-8000-000000000002', '019b4000-0000-7000-8000-000000000001', 'Bart', 'CiQwMTliNDAwMC0xMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDISBWxvY2Fs', 'viewer', 'bart@acme-corp.com'),
    ('019b4000-1000-7000-8000-000000000003', '019b4000-0000-7000-8000-000000000001', 'Cindy', 'CiQwMTliNDAwMC0xMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDMSBWxvY2Fs', 'viewer', 'cindy@acme-corp.com'),
    -- globex users
    ('019b4000-1000-7000-8000-000000000004', '019b4000-0000-7000-8000-000000000002', 'David', 'CiQwMTliNDAwMC0xMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDQSBWxvY2Fs', 'admin', 'david@globex.com'),
    ('019b4000-1000-7000-8000-000000000005', '019b4000-0000-7000-8000-000000000002', 'Emily', 'CiQwMTliNDAwMC0xMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDUSBWxvY2Fs', 'viewer', 'emily@globex.com'),
    -- initech users
    ('019b4000-1000-7000-8000-000000000006', '019b4000-0000-7000-8000-000000000003', 'Frank', 'CiQwMTliNDAwMC0xMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDYSBWxvY2Fs', 'admin', 'frank@initech.com'),
    ('019b4000-1000-7000-8000-000000000007', '019b4000-0000-7000-8000-000000000003', 'Grace', 'CiQwMTliNDAwMC0xMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDcSBWxvY2Fs', 'viewer', 'grace@initech.com');

-- Cluster for acme-corp
INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version) VALUES
    ('019b4000-2000-7000-8000-000000000001', '019b4000-0000-7000-8000-000000000001', 'acme-cluster', 'eu-west-1', '1.31.0');
