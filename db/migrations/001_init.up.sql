CREATE SCHEMA organization;


CREATE TABLE organization.tenants (
    id uuid NOT NULL DEFAULT uuidv7(),
    name text not null,
    created timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX tenants_pk ON organization.tenants USING btree (id);

ALTER TABLE organization.tenants ADD CONSTRAINT tenants_pk PRIMARY KEY USING INDEX tenants_pk;

CREATE UNIQUE INDEX tenants_uq_name ON organization.tenants USING btree (name);

ALTER TABLE organization.tenants ADD CONSTRAINT tenants_uq_name UNIQUE USING INDEX tenants_uq_name;


CREATE TABLE organization.projects (
    id uuid NOT NULL DEFAULT uuidv7(),
    tenant_id uuid not null,
    name text not null,
    created timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX projects_pk ON organization.projects USING btree (id);

ALTER TABLE organization.projects ADD CONSTRAINT projects_pk PRIMARY KEY USING INDEX projects_pk;

ALTER TABLE organization.projects ADD CONSTRAINT projects_fk_tenant FOREIGN KEY (tenant_id) REFERENCES organization.tenants(id);

CREATE UNIQUE INDEX projects_uq_tenant_name ON organization.projects USING btree (tenant_id, name);

ALTER TABLE organization.projects ADD CONSTRAINT projects_uq_tenant_name UNIQUE USING INDEX projects_uq_tenant_name;


CREATE TABLE organization.clusters (
    id uuid NOT NULL DEFAULT uuidv7(),
    tenant_id uuid not null,
    name text not null,
    created timestamptz NOT NULL DEFAULT now(),
    deleted timestamptz
);

CREATE UNIQUE INDEX clusters_pk ON organization.clusters USING btree (id);

ALTER TABLE organization.clusters ADD CONSTRAINT clusters_pk PRIMARY KEY USING INDEX clusters_pk;

CREATE UNIQUE INDEX clusters_uq_name ON organization.clusters USING btree (tenant_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE organization.clusters ADD CONSTRAINT clusters_fk_tenant FOREIGN KEY (tenant_id) REFERENCES organization.tenants(id);

ALTER TABLE organization.clusters ADD CONSTRAINT clusters_uq_name UNIQUE USING INDEX clusters_uq_name;


CREATE TABLE organization.namespaces (
    id uuid NOT NULL DEFAULT uuidv7(),
    project_id uuid not null,
    cluster_id uuid not null,
    name text not null,
    created timestamptz NOT NULL DEFAULT now(),
    deleted timestamptz
);

CREATE UNIQUE INDEX namespaces_pk ON organization.namespaces USING btree (id);

ALTER TABLE organization.namespaces ADD CONSTRAINT namespaces_pk PRIMARY KEY USING INDEX namespaces_pk;

CREATE UNIQUE INDEX namespaces_uq_name ON organization.namespaces USING btree (project_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE organization.namespaces ADD CONSTRAINT namespaces_uq_name UNIQUE USING INDEX namespaces_uq_name;

ALTER TABLE organization.namespaces ADD CONSTRAINT namespaces_ck_name CHECK ((name = name));

ALTER TABLE organization.namespaces ADD CONSTRAINT namespaces_fk_cluster FOREIGN KEY (cluster_id) REFERENCES organization.clusters(id);

ALTER TABLE organization.namespaces ADD CONSTRAINT namespaces_fk_project FOREIGN KEY (project_id) REFERENCES organization.projects(id);


CREATE OR REPLACE FUNCTION organization.clusters_tr_verify_deleted()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM organization.namespaces
		WHERE
			cluster_id = NEW.id
			AND deleted IS NULL
	) THEN
		RAISE EXCEPTION 'Cannot delete cluster with undeleted namespaces';
	END IF;
END;
$function$
;

CREATE CONSTRAINT TRIGGER verify_deleted
AFTER INSERT OR UPDATE
ON organization.clusters
NOT DEFERRABLE INITIALLY IMMEDIATE
FOR EACH ROW
EXECUTE FUNCTION organization.clusters_tr_verify_deleted();
