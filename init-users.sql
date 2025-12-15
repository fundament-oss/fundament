CREATE USER authn_api WITH PASSWORD 'password';
CREATE USER appstore_api WITH PASSWORD 'password';
CREATE USER organization_api WITH PASSWORD 'password';

GRANT ALL PRIVILEGES ON DATABASE fundament TO authn_api;
GRANT USAGE ON SCHEMA organization TO authn_api;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA organization TO authn_api;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA organization TO authn_api;

GRANT ALL PRIVILEGES ON DATABASE fundament TO appstore_api;
GRANT USAGE ON SCHEMA organization TO appstore_api;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA organization TO appstore_api;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA organization TO appstore_api;

GRANT ALL PRIVILEGES ON DATABASE fundament TO organization_api;
GRANT USAGE ON SCHEMA organization TO organization_api;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA organization TO organization_api;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA organization TO organization_api;