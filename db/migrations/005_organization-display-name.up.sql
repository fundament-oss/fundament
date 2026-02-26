
SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE tenant.organizations ADD COLUMN display_name text;

UPDATE tenant.organizations SET display_name = name;

ALTER TABLE tenant.organizations ALTER COLUMN display_name SET NOT NULL;

ALTER TABLE tenant.organizations ADD CONSTRAINT organizations_ck_display_name CHECK (
	char_length(display_name) >= 1
	AND char_length(display_name) <= 255
) NOT VALID;

ALTER TABLE tenant.organizations VALIDATE CONSTRAINT organizations_ck_display_name;

ALTER TABLE tenant.organizations ADD CONSTRAINT organizations_ck_name CHECK (
	name ~ '^[a-z][a-z0-9-]*[a-z0-9]$'
) NOT VALID;

ALTER TABLE tenant.organizations VALIDATE CONSTRAINT organizations_ck_name;
