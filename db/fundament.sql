-- ** Database generated with pgModeler (PostgreSQL Database Modeler).
-- ** pgModeler version: 1.2.2
-- ** PostgreSQL version: 18.0
-- ** Project Site: pgmodeler.io
-- ** Model Author: ---

SET check_function_bodies = false;
-- ddl-end --

-- object: tenant | type: SCHEMA --
-- DROP SCHEMA IF EXISTS tenant CASCADE;
CREATE SCHEMA tenant;
-- ddl-end --
ALTER SCHEMA tenant OWNER TO postgres;
-- ddl-end --

SET search_path TO pg_catalog,public,tenant;
-- ddl-end --

-- object: tenant.organizations | type: TABLE --
-- DROP TABLE IF EXISTS tenant.organizations CASCADE;
CREATE TABLE tenant.organizations (
	id uuid NOT NULL DEFAULT uuidv7(),
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT organizations_pk PRIMARY KEY (id),
	CONSTRAINT organizations_uq_name UNIQUE (name)
);
-- ddl-end --
ALTER TABLE tenant.organizations OWNER TO postgres;
-- ddl-end --

-- object: tenant.projects | type: TABLE --
-- DROP TABLE IF EXISTS tenant.projects CASCADE;
CREATE TABLE tenant.projects (
	id uuid NOT NULL DEFAULT uuidv7(),
	organization_id uuid NOT NULL,
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT projects_pk PRIMARY KEY (id),
	CONSTRAINT projects_uq_organization_name UNIQUE (organization_id,name)
);
-- ddl-end --
ALTER TABLE tenant.projects OWNER TO postgres;
-- ddl-end --

-- object: tenant.namespaces | type: TABLE --
-- DROP TABLE IF EXISTS tenant.namespaces CASCADE;
CREATE TABLE tenant.namespaces (
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
ALTER TABLE tenant.namespaces OWNER TO postgres;
-- ddl-end --

-- object: tenant.clusters_tr_verify_deleted | type: FUNCTION --
-- DROP FUNCTION IF EXISTS tenant.clusters_tr_verify_deleted() CASCADE;
CREATE OR REPLACE FUNCTION tenant.clusters_tr_verify_deleted ()
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
		FROM tenant.namespaces
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
ALTER FUNCTION tenant.clusters_tr_verify_deleted() OWNER TO postgres;
-- ddl-end --

-- object: tenant.users | type: TABLE --
-- DROP TABLE IF EXISTS tenant.users CASCADE;
CREATE TABLE tenant.users (
	id uuid NOT NULL DEFAULT uuidv7(),
	organization_id uuid NOT NULL,
	name text NOT NULL,
	external_id text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT users_pk PRIMARY KEY (id),
	CONSTRAINT users_uq_external_id UNIQUE (external_id)
);
-- ddl-end --
ALTER TABLE tenant.users OWNER TO postgres;
-- ddl-end --

-- object: tenant.clusters | type: TABLE --
-- DROP TABLE IF EXISTS tenant.clusters CASCADE;
CREATE TABLE tenant.clusters (
	id uuid NOT NULL DEFAULT uuidv7(),
	organization_id uuid NOT NULL,
	name text NOT NULL,
	region text NOT NULL,
	kubernetes_version text NOT NULL,
	status text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT clusters_pk PRIMARY KEY (id),
	CONSTRAINT clusters_uq_name UNIQUE NULLS NOT DISTINCT (organization_id,name,deleted),
	CONSTRAINT clusters_ck_status CHECK (status IN ('unspecified','provisioning','starting','running','upgrading','error','stopping','stopped'))
);
-- ddl-end --
ALTER TABLE tenant.clusters OWNER TO postgres;
-- ddl-end --
ALTER TABLE tenant.clusters ENABLE ROW LEVEL SECURITY;
-- ddl-end --

-- object: verify_deleted | type: TRIGGER --
-- verify_deleted ON tenant.clusters CASCADE;
CREATE CONSTRAINT TRIGGER verify_deleted
	AFTER INSERT OR UPDATE
	ON tenant.clusters
	NOT DEFERRABLE 
	FOR EACH ROW
	EXECUTE PROCEDURE tenant.clusters_tr_verify_deleted();
-- ddl-end --

-- object: organization_isolation | type: POLICY --
-- DROP POLICY IF EXISTS organization_isolation ON tenant.clusters CASCADE;
CREATE POLICY organization_isolation ON tenant.clusters
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (organization_id = current_setting('app.current_organization_id')::uuid);
-- ddl-end --

-- object: tenant.node_pools | type: TABLE --
-- DROP TABLE IF EXISTS tenant.node_pools CASCADE;
CREATE TABLE tenant.node_pools (
	id uuid NOT NULL DEFAULT uuidv7(),
	cluster_id uuid NOT NULL,
	name text NOT NULL,
	machine_type text NOT NULL,
	autoscale_min integer NOT NULL,
	autoscale_max integer NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT node_pools_pk PRIMARY KEY (id),
	CONSTRAINT node_pools_uq_name UNIQUE NULLS NOT DISTINCT (cluster_id,name,deleted)
);
-- ddl-end --
ALTER TABLE tenant.node_pools OWNER TO postgres;
-- ddl-end --
ALTER TABLE tenant.node_pools ENABLE ROW LEVEL SECURITY;
-- ddl-end --

-- object: node_pools_organization_policy | type: POLICY --
-- DROP POLICY IF EXISTS node_pools_organization_policy ON tenant.node_pools CASCADE;
CREATE POLICY node_pools_organization_policy ON tenant.node_pools
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (EXISTS (
      SELECT 1 FROM clusters
      WHERE clusters.id = node_pools.cluster_id
      AND clusters.organization_id = current_setting('app.current_organization_id')::uuid
));
-- ddl-end --

-- object: projects_fk_organization | type: CONSTRAINT --
-- ALTER TABLE tenant.projects DROP CONSTRAINT IF EXISTS projects_fk_organization CASCADE;
ALTER TABLE tenant.projects ADD CONSTRAINT projects_fk_organization FOREIGN KEY (organization_id)
REFERENCES tenant.organizations (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: namespaces_fk_project | type: CONSTRAINT --
-- ALTER TABLE tenant.namespaces DROP CONSTRAINT IF EXISTS namespaces_fk_project CASCADE;
ALTER TABLE tenant.namespaces ADD CONSTRAINT namespaces_fk_project FOREIGN KEY (project_id)
REFERENCES tenant.projects (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: namespaces_fk_cluster | type: CONSTRAINT --
-- ALTER TABLE tenant.namespaces DROP CONSTRAINT IF EXISTS namespaces_fk_cluster CASCADE;
ALTER TABLE tenant.namespaces ADD CONSTRAINT namespaces_fk_cluster FOREIGN KEY (cluster_id)
REFERENCES tenant.clusters (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: users_fk_organization | type: CONSTRAINT --
-- ALTER TABLE tenant.users DROP CONSTRAINT IF EXISTS users_fk_organization CASCADE;
ALTER TABLE tenant.users ADD CONSTRAINT users_fk_organization FOREIGN KEY (organization_id)
REFERENCES tenant.organizations (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: clusters_fk_organization | type: CONSTRAINT --
-- ALTER TABLE tenant.clusters DROP CONSTRAINT IF EXISTS clusters_fk_organization CASCADE;
ALTER TABLE tenant.clusters ADD CONSTRAINT clusters_fk_organization FOREIGN KEY (organization_id)
REFERENCES tenant.organizations (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: node_pools_fk_cluster | type: CONSTRAINT --
-- ALTER TABLE tenant.node_pools DROP CONSTRAINT IF EXISTS node_pools_fk_cluster CASCADE;
ALTER TABLE tenant.node_pools ADD CONSTRAINT node_pools_fk_cluster FOREIGN KEY (cluster_id)
REFERENCES tenant.clusters (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --


