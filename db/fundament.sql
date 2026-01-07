-- ** Database generated with pgModeler (PostgreSQL Database Modeler).
-- ** pgModeler version: 1.2.2
-- ** PostgreSQL version: 18.0
-- ** Project Site: pgmodeler.io
-- ** Model Author: ---

SET check_function_bodies = false;
-- ddl-end --

-- object: organization | type: SCHEMA --
-- DROP SCHEMA IF EXISTS organization CASCADE;
CREATE SCHEMA organization;
-- ddl-end --
ALTER SCHEMA organization OWNER TO postgres;
-- ddl-end --

SET search_path TO pg_catalog,public,organization;
-- ddl-end --

-- object: organization.tenants | type: TABLE --
-- DROP TABLE IF EXISTS organization.tenants CASCADE;
CREATE TABLE organization.tenants (
	id uuid NOT NULL DEFAULT uuidv7(),
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT tenants_pk PRIMARY KEY (id),
	CONSTRAINT tenants_uq_name UNIQUE (name)
);
-- ddl-end --
ALTER TABLE organization.tenants OWNER TO postgres;
-- ddl-end --

-- object: organization.projects | type: TABLE --
-- DROP TABLE IF EXISTS organization.projects CASCADE;
CREATE TABLE organization.projects (
	id uuid NOT NULL DEFAULT uuidv7(),
	tenant_id uuid NOT NULL,
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT projects_pk PRIMARY KEY (id),
	CONSTRAINT projects_uq_tenant_name UNIQUE (tenant_id,name)
);
-- ddl-end --
ALTER TABLE organization.projects OWNER TO postgres;
-- ddl-end --

-- object: organization.namespaces | type: TABLE --
-- DROP TABLE IF EXISTS organization.namespaces CASCADE;
CREATE TABLE organization.namespaces (
	id uuid NOT NULL DEFAULT uuidv7(),
	project_id uuid NOT NULL,
	cluster_id uuid NOT NULL,
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT namespaces_pk PRIMARY KEY (id),
	CONSTRAINT namespaces_ck_name CHECK (name = name),
	CONSTRAINT namespaces_uq_name UNIQUE NULLS NOT DISTINCT (project_id,name,deleted)
);
-- ddl-end --
ALTER TABLE organization.namespaces OWNER TO postgres;
-- ddl-end --

-- object: organization.clusters_tr_verify_deleted | type: FUNCTION --
-- DROP FUNCTION IF EXISTS organization.clusters_tr_verify_deleted() CASCADE;
CREATE OR REPLACE FUNCTION organization.clusters_tr_verify_deleted ()
	RETURNS trigger
	LANGUAGE plpgsql
	VOLATILE 
	CALLED ON NULL INPUT
	SECURITY INVOKER
	PARALLEL UNSAFE
	COST 1
	AS 
$function$
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
	RETURN NEW;
END;
$function$;
-- ddl-end --
ALTER FUNCTION organization.clusters_tr_verify_deleted() OWNER TO postgres;
-- ddl-end --

-- object: organization.users | type: TABLE --
-- DROP TABLE IF EXISTS organization.users CASCADE;
CREATE TABLE organization.users (
	id uuid NOT NULL DEFAULT uuidv7(),
	tenant_id uuid NOT NULL,
	name text NOT NULL,
	external_id text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT users_pk PRIMARY KEY (id),
	CONSTRAINT users_uq_external_id UNIQUE (external_id)
);
-- ddl-end --
ALTER TABLE organization.users OWNER TO postgres;
-- ddl-end --

-- object: organization.clusters | type: TABLE --
-- DROP TABLE IF EXISTS organization.clusters CASCADE;
CREATE TABLE organization.clusters (
	id uuid NOT NULL DEFAULT uuidv7(),
	tenant_id uuid NOT NULL,
	name text NOT NULL,
	region text NOT NULL,
	kubernetes_version text NOT NULL,
	status text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT clusters_pk PRIMARY KEY (id),
	CONSTRAINT clusters_uq_name UNIQUE NULLS NOT DISTINCT (tenant_id,name,deleted),
	CONSTRAINT clusters_ck_status CHECK (status IN ('unspecified','provisioning','starting','running','upgrading','error','stopping','stopped')) NO INHERIT
);
-- ddl-end --
ALTER TABLE organization.clusters OWNER TO postgres;
-- ddl-end --
ALTER TABLE organization.clusters ENABLE ROW LEVEL SECURITY;
-- ddl-end --

-- object: verify_deleted | type: TRIGGER --
-- verify_deleted ON organization.clusters CASCADE;
CREATE CONSTRAINT TRIGGER verify_deleted
	AFTER INSERT OR UPDATE
	ON organization.clusters
	NOT DEFERRABLE 
	FOR EACH ROW
	EXECUTE PROCEDURE organization.clusters_tr_verify_deleted();
-- ddl-end --

-- object: tenant_isolation | type: POLICY --
-- DROP POLICY IF EXISTS tenant_isolation ON organization.clusters CASCADE;
CREATE POLICY tenant_isolation ON organization.clusters
	AS PERMISSIVE
	FOR ALL
	TO fun_organization_api
	USING (tenant_id = current_setting('app.current_tenant_id')::uuid);
-- ddl-end --

-- object: projects_fk_tenant | type: CONSTRAINT --
-- ALTER TABLE organization.projects DROP CONSTRAINT IF EXISTS projects_fk_tenant CASCADE;
ALTER TABLE organization.projects ADD CONSTRAINT projects_fk_tenant FOREIGN KEY (tenant_id)
REFERENCES organization.tenants (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: namespaces_fk_project | type: CONSTRAINT --
-- ALTER TABLE organization.namespaces DROP CONSTRAINT IF EXISTS namespaces_fk_project CASCADE;
ALTER TABLE organization.namespaces ADD CONSTRAINT namespaces_fk_project FOREIGN KEY (project_id)
REFERENCES organization.projects (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: namespaces_fk_cluster | type: CONSTRAINT --
-- ALTER TABLE organization.namespaces DROP CONSTRAINT IF EXISTS namespaces_fk_cluster CASCADE;
ALTER TABLE organization.namespaces ADD CONSTRAINT namespaces_fk_cluster FOREIGN KEY (cluster_id)
REFERENCES organization.clusters (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: users_fk_tenant | type: CONSTRAINT --
-- ALTER TABLE organization.users DROP CONSTRAINT IF EXISTS users_fk_tenant CASCADE;
ALTER TABLE organization.users ADD CONSTRAINT users_fk_tenant FOREIGN KEY (tenant_id)
REFERENCES organization.tenants (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: clusters_fk_tenant | type: CONSTRAINT --
-- ALTER TABLE organization.clusters DROP CONSTRAINT IF EXISTS clusters_fk_tenant CASCADE;
ALTER TABLE organization.clusters ADD CONSTRAINT clusters_fk_tenant FOREIGN KEY (tenant_id)
REFERENCES organization.tenants (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --


