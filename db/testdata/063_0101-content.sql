-- A known API key for Alice in acme-corp (TREK_INSERT_TEST_DATA only), so a
-- fresh local environment can be driven by functl, the OpenTofu provider and
-- scripts without creating a key in the console first:
--
--   functl auth login fun_LocalDevAliceAcmeCorpKey0000004KyU0e
--
-- Only the SHA-256 hash is stored, as for a key created through the API. The
-- token is a valid fun_ token (its last six characters are the CRC32 checksum
-- apitoken.ValidateFormat checks), so changing it means recomputing that
-- checksum. The api_keys_outbox trigger queues the key for the authz-worker,
-- which writes its OpenFGA tuples like for any other key. No expiry: the
-- database is reset on every deploy.

INSERT INTO authn.api_keys (id, organization_id, user_id, name, token_hash, token_prefix) VALUES
    ('019b4000-b000-7000-8000-000000000001', '019b4000-0000-7000-8000-000000000001', '019b4000-1000-7000-8000-000000000001',
     'local-dev', sha256(convert_to('fun_LocalDevAliceAcmeCorpKey0000004KyU0e', 'UTF8')), 'fun_Loca');
