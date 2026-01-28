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
ALTER SCHEMA tenant OWNER TO fun_owner;
-- ddl-end --

-- object: zappstore | type: SCHEMA --
-- DROP SCHEMA IF EXISTS zappstore CASCADE;
CREATE SCHEMA zappstore;
-- ddl-end --
ALTER SCHEMA zappstore OWNER TO fun_owner;
-- ddl-end --

SET search_path TO pg_catalog,public,tenant,zappstore;
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
ALTER TABLE tenant.organizations OWNER TO fun_owner;
-- ddl-end --

-- object: tenant.projects | type: TABLE --
-- DROP TABLE IF EXISTS tenant.projects CASCADE;
CREATE TABLE tenant.projects (
	id uuid NOT NULL DEFAULT uuidv7(),
	organization_id uuid NOT NULL,
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT projects_pk PRIMARY KEY (id),
	CONSTRAINT projects_uq_organization_name UNIQUE NULLS NOT DISTINCT (organization_id,name,deleted)
);
-- ddl-end --
ALTER TABLE tenant.projects OWNER TO fun_owner;
-- ddl-end --
ALTER TABLE tenant.projects ENABLE ROW LEVEL SECURITY;
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
ALTER TABLE tenant.namespaces OWNER TO fun_owner;
-- ddl-end --
ALTER TABLE tenant.namespaces ENABLE ROW LEVEL SECURITY;
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

-- object: tenant.cluster_reset_synced | type: FUNCTION --
-- DROP FUNCTION IF EXISTS tenant.cluster_reset_synced() CASCADE;
CREATE OR REPLACE FUNCTION tenant.cluster_reset_synced ()
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
    NEW.synced := NULL;
    NEW.sync_claimed_at := NULL;
    NEW.sync_attempts := 0;
    NEW.sync_error := NULL;
    RETURN NEW;
END;
$function$;
-- ddl-end --
ALTER FUNCTION tenant.cluster_reset_synced() OWNER TO postgres;
-- ddl-end --

-- object: tenant.cluster_sync_notify | type: FUNCTION --
-- DROP FUNCTION IF EXISTS tenant.cluster_sync_notify() CASCADE;
CREATE OR REPLACE FUNCTION tenant.cluster_sync_notify ()
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
    IF NEW.synced IS NULL AND (TG_OP = 'INSERT' OR OLD.synced IS NOT NULL) THEN
        PERFORM pg_notify('cluster_sync', '');
    END IF;
    RETURN NEW;
END;
$function$;
-- ddl-end --
ALTER FUNCTION tenant.cluster_sync_notify() OWNER TO postgres;
-- ddl-end --

-- object: tenant.users | type: TABLE --
-- DROP TABLE IF EXISTS tenant.users CASCADE;
CREATE TABLE tenant.users (
	id uuid NOT NULL DEFAULT uuidv7(),
	organization_id uuid NOT NULL,
	name text NOT NULL,
	external_id text,
	created timestamptz NOT NULL DEFAULT now(),
	email text,
	role text NOT NULL DEFAULT 'viewer',
	deleted timestamptz,
	CONSTRAINT users_pk PRIMARY KEY (id),
	CONSTRAINT users_uq_external_id UNIQUE NULLS NOT DISTINCT (external_id,deleted)
);
-- ddl-end --
ALTER TABLE tenant.users OWNER TO fun_owner;
-- ddl-end --

-- object: tenant.clusters | type: TABLE --
-- DROP TABLE IF EXISTS tenant.clusters CASCADE;
CREATE TABLE tenant.clusters (
	id uuid NOT NULL DEFAULT uuidv7(),
	organization_id uuid NOT NULL,
	name text NOT NULL,
	region text NOT NULL,
	kubernetes_version text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	synced timestamptz,
	sync_claimed_at timestamptz,
	sync_error text,
	sync_attempts integer NOT NULL DEFAULT 0,
	shoot_status text,
	shoot_status_message text,
	shoot_status_updated timestamptz,
	CONSTRAINT clusters_pk PRIMARY KEY (id),
	CONSTRAINT clusters_uq_name UNIQUE NULLS NOT DISTINCT (organization_id,name,deleted)
);
-- ddl-end --
ALTER TABLE tenant.clusters OWNER TO fun_owner;
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

-- object: cluster_worker_all_access | type: POLICY --
-- DROP POLICY IF EXISTS cluster_worker_all_access ON tenant.clusters CASCADE;
CREATE POLICY cluster_worker_all_access ON tenant.clusters
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);
-- ddl-end --

-- object: cluster_reset_synced | type: TRIGGER --
-- DROP TRIGGER IF EXISTS cluster_reset_synced ON tenant.clusters CASCADE;
CREATE OR REPLACE TRIGGER cluster_reset_synced
	BEFORE UPDATE OF name,region,kubernetes_version,deleted
	ON tenant.clusters
	FOR EACH ROW
	WHEN (OLD.name IS DISTINCT FROM NEW.name
    OR OLD.region IS DISTINCT FROM NEW.region
    OR OLD.kubernetes_version IS DISTINCT FROM NEW.kubernetes_version
    OR (OLD.deleted IS NULL AND NEW.deleted IS NOT NULL))
	EXECUTE PROCEDURE tenant.cluster_reset_synced();
-- ddl-end --

-- object: cluster_sync_notify | type: TRIGGER --
-- DROP TRIGGER IF EXISTS cluster_sync_notify ON tenant.clusters CASCADE;
CREATE OR REPLACE TRIGGER cluster_sync_notify
	AFTER INSERT OR UPDATE OF synced
	ON tenant.clusters
	FOR EACH ROW
	EXECUTE PROCEDURE tenant.cluster_sync_notify();
-- ddl-end --

-- object: clusters_idx_needs_sync | type: INDEX --
-- DROP INDEX IF EXISTS tenant.clusters_idx_needs_sync CASCADE;
CREATE INDEX clusters_idx_needs_sync ON tenant.clusters
USING btree
(
	created
)
WHERE (synced IS NULL);
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
ALTER TABLE tenant.node_pools OWNER TO fun_owner;
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

-- object: zappstore.installs | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.installs CASCADE;
CREATE TABLE zappstore.installs (
	id uuid NOT NULL DEFAULT uuidv7(),
	cluster_id uuid NOT NULL,
	plugin_id uuid NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT installs_pk PRIMARY KEY (id),
	CONSTRAINT installs_uq UNIQUE NULLS NOT DISTINCT (cluster_id,plugin_id,deleted)
);
-- ddl-end --
ALTER TABLE zappstore.installs OWNER TO fun_owner;
-- ddl-end --
ALTER TABLE zappstore.installs ENABLE ROW LEVEL SECURITY;
-- ddl-end --

-- object: zappstore.plugins | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.plugins CASCADE;
CREATE TABLE zappstore.plugins (
	id uuid NOT NULL DEFAULT uuidv7(),
	name text NOT NULL,
	description_short text NOT NULL DEFAULT '',
	description text NOT NULL,
	author_name text,
	author_url text,
	repository_url text,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT plugins_uq_name UNIQUE NULLS NOT DISTINCT (name,deleted),
	CONSTRAINT plugins_pk PRIMARY KEY (id)
);
-- ddl-end --
ALTER TABLE zappstore.plugins OWNER TO fun_owner;
-- ddl-end --

-- object: zappstore.presets | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.presets CASCADE;
CREATE TABLE zappstore.presets (
	id uuid NOT NULL DEFAULT uuidv7(),
	name text NOT NULL,
	description text,
	CONSTRAINT presets_pk PRIMARY KEY (id),
	CONSTRAINT presets_uq_name UNIQUE (name)
);
-- ddl-end --
ALTER TABLE zappstore.presets OWNER TO fun_owner;
-- ddl-end --

-- object: zappstore.preset_plugins | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.preset_plugins CASCADE;
CREATE TABLE zappstore.preset_plugins (
	preset_id uuid NOT NULL,
	plugin_id uuid NOT NULL,
	CONSTRAINT preset_plugins_pk PRIMARY KEY (preset_id,plugin_id)
);
-- ddl-end --
ALTER TABLE zappstore.preset_plugins OWNER TO fun_owner;
-- ddl-end --

-- object: install_organization_policy | type: POLICY --
-- DROP POLICY IF EXISTS install_organization_policy ON zappstore.installs CASCADE;
CREATE POLICY install_organization_policy ON zappstore.installs
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (EXISTS (
      SELECT 1 FROM clusters
      WHERE clusters.id = installs.cluster_id
      AND clusters.organization_id = current_setting('app.current_organization_id')::uuid
));
-- ddl-end --

-- object: zappstore.tags | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.tags CASCADE;
CREATE TABLE zappstore.tags (
	id uuid NOT NULL DEFAULT uuidv7(),
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT tags_uq_name UNIQUE NULLS NOT DISTINCT (name,deleted),
	CONSTRAINT tags_pk PRIMARY KEY (id)
);
-- ddl-end --
ALTER TABLE zappstore.tags OWNER TO fun_owner;
-- ddl-end --

-- object: zappstore.plugins_tags | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.plugins_tags CASCADE;
CREATE TABLE zappstore.plugins_tags (
	plugin_id uuid NOT NULL,
	tag_id uuid NOT NULL,
	CONSTRAINT plugins_tags_pk PRIMARY KEY (plugin_id,tag_id)
);
-- ddl-end --
ALTER TABLE zappstore.plugins_tags OWNER TO fun_owner;
-- ddl-end --

-- object: zappstore.categories | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.categories CASCADE;
CREATE TABLE zappstore.categories (
	id uuid NOT NULL DEFAULT uuidv7(),
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT categories_uq_name UNIQUE NULLS NOT DISTINCT (name,deleted),
	CONSTRAINT categories_pk PRIMARY KEY (id)
);
-- ddl-end --
ALTER TABLE zappstore.categories OWNER TO fun_owner;
-- ddl-end --

-- object: zappstore.categories_plugins | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.categories_plugins CASCADE;
CREATE TABLE zappstore.categories_plugins (
	plugin_id uuid NOT NULL,
	category_id uuid NOT NULL,
	CONSTRAINT categories_plugins_pk PRIMARY KEY (plugin_id,category_id)
);
-- ddl-end --
ALTER TABLE zappstore.categories_plugins OWNER TO fun_owner;
-- ddl-end --

-- object: zappstore.plugin_documentation_links | type: TABLE --
-- DROP TABLE IF EXISTS zappstore.plugin_documentation_links CASCADE;
CREATE TABLE zappstore.plugin_documentation_links (
	id uuid NOT NULL DEFAULT uuidv7(),
	plugin_id uuid NOT NULL,
	title text NOT NULL,
	url_name text NOT NULL,
	url text NOT NULL,
	CONSTRAINT plugin_documentation_links_pk PRIMARY KEY (id)
);
-- ddl-end --
ALTER TABLE zappstore.plugin_documentation_links OWNER TO fun_owner;
-- ddl-end --

-- object: projects_organization_isolation | type: POLICY --
-- DROP POLICY IF EXISTS projects_organization_isolation ON tenant.projects CASCADE;
CREATE POLICY projects_organization_isolation ON tenant.projects
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (organization_id = current_setting('app.current_organization_id')::uuid);
-- ddl-end --

-- object: namespaces_organization_policy | type: POLICY --
-- DROP POLICY IF EXISTS namespaces_organization_policy ON tenant.namespaces CASCADE;
CREATE POLICY namespaces_organization_policy ON tenant.namespaces
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (EXISTS (
    SELECT 1 FROM clusters
    WHERE clusters.id = namespaces.cluster_id
    AND clusters.organization_id = current_setting('app.current_organization_id')::uuid
));
-- ddl-end --

-- object: tenant.cluster_events | type: TABLE --
-- DROP TABLE IF EXISTS tenant.cluster_events CASCADE;
CREATE TABLE tenant.cluster_events (
	id uuid NOT NULL DEFAULT uuidv7(),
	cluster_id uuid NOT NULL,
	event_type text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	sync_action text,
	message text,
	attempt integer,
	CONSTRAINT cluster_events_pk PRIMARY KEY (id),
	CONSTRAINT cluster_events_ck_event_type CHECK (event_type IN ('sync_requested','sync_claimed','sync_succeeded','sync_failed','status_progressing','status_ready','status_error','status_deleted')),
	CONSTRAINT cluster_events_ck_sync_action CHECK (sync_action IN ('sync','delete'))
);
-- ddl-end --
ALTER TABLE tenant.cluster_events OWNER TO postgres;
-- ddl-end --
ALTER TABLE tenant.cluster_events ENABLE ROW LEVEL SECURITY;
-- ddl-end --

-- object: cluster_events_worker_all_access | type: POLICY --
-- DROP POLICY IF EXISTS cluster_events_worker_all_access ON tenant.cluster_events CASCADE;
CREATE POLICY cluster_events_worker_all_access ON tenant.cluster_events
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);
-- ddl-end --

-- object: cluster_events_organization_isolation | type: POLICY --
-- DROP POLICY IF EXISTS cluster_events_organization_isolation ON tenant.cluster_events CASCADE;
CREATE POLICY cluster_events_organization_isolation ON tenant.cluster_events
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (EXISTS (
    SELECT 1 FROM tenant.clusters c
    WHERE c.id = cluster_events.cluster_id
    AND c.organization_id = current_setting('app.current_organization_id')::uuid
));
-- ddl-end --

-- object: cluster_events_idx_cluster_created | type: INDEX --
-- DROP INDEX IF EXISTS tenant.cluster_events_idx_cluster_created CASCADE;
CREATE INDEX cluster_events_idx_cluster_created ON tenant.cluster_events
USING btree
(
	cluster_id DESC NULLS LAST,
	created DESC NULLS LAST
);
-- ddl-end --

-- object: projects_fk_organization | type: CONSTRAINT --
-- ALTER TABLE tenant.projects DROP CONSTRAINT IF EXISTS projects_fk_organization CASCADE;
ALTER TABLE tenant.projects ADD CONSTRAINT projects_fk_organization FOREIGN KEY (organization_id)
REFERENCES tenant.organizations (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: namespaces_fk_cluster | type: CONSTRAINT --
-- ALTER TABLE tenant.namespaces DROP CONSTRAINT IF EXISTS namespaces_fk_cluster CASCADE;
ALTER TABLE tenant.namespaces ADD CONSTRAINT namespaces_fk_cluster FOREIGN KEY (cluster_id)
REFERENCES tenant.clusters (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: namespaces_fk_project | type: CONSTRAINT --
-- ALTER TABLE tenant.namespaces DROP CONSTRAINT IF EXISTS namespaces_fk_project CASCADE;
ALTER TABLE tenant.namespaces ADD CONSTRAINT namespaces_fk_project FOREIGN KEY (project_id)
REFERENCES tenant.projects (id) MATCH SIMPLE
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

-- object: installs_fk_cluster | type: CONSTRAINT --
-- ALTER TABLE zappstore.installs DROP CONSTRAINT IF EXISTS installs_fk_cluster CASCADE;
ALTER TABLE zappstore.installs ADD CONSTRAINT installs_fk_cluster FOREIGN KEY (cluster_id)
REFERENCES tenant.clusters (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: installs_fk_plugin | type: CONSTRAINT --
-- ALTER TABLE zappstore.installs DROP CONSTRAINT IF EXISTS installs_fk_plugin CASCADE;
ALTER TABLE zappstore.installs ADD CONSTRAINT installs_fk_plugin FOREIGN KEY (plugin_id)
REFERENCES zappstore.plugins (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: plugins_presets_plugin_id | type: CONSTRAINT --
-- ALTER TABLE zappstore.preset_plugins DROP CONSTRAINT IF EXISTS plugins_presets_plugin_id CASCADE;
ALTER TABLE zappstore.preset_plugins ADD CONSTRAINT plugins_presets_plugin_id FOREIGN KEY (plugin_id)
REFERENCES zappstore.plugins (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: plugins_presets_preset_id | type: CONSTRAINT --
-- ALTER TABLE zappstore.preset_plugins DROP CONSTRAINT IF EXISTS plugins_presets_preset_id CASCADE;
ALTER TABLE zappstore.preset_plugins ADD CONSTRAINT plugins_presets_preset_id FOREIGN KEY (preset_id)
REFERENCES zappstore.presets (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: plugins_tags_tag_id | type: CONSTRAINT --
-- ALTER TABLE zappstore.plugins_tags DROP CONSTRAINT IF EXISTS plugins_tags_tag_id CASCADE;
ALTER TABLE zappstore.plugins_tags ADD CONSTRAINT plugins_tags_tag_id FOREIGN KEY (tag_id)
REFERENCES zappstore.tags (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: plugins_tags_plugin_id | type: CONSTRAINT --
-- ALTER TABLE zappstore.plugins_tags DROP CONSTRAINT IF EXISTS plugins_tags_plugin_id CASCADE;
ALTER TABLE zappstore.plugins_tags ADD CONSTRAINT plugins_tags_plugin_id FOREIGN KEY (plugin_id)
REFERENCES zappstore.plugins (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: plugins_categories_plugin_id | type: CONSTRAINT --
-- ALTER TABLE zappstore.categories_plugins DROP CONSTRAINT IF EXISTS plugins_categories_plugin_id CASCADE;
ALTER TABLE zappstore.categories_plugins ADD CONSTRAINT plugins_categories_plugin_id FOREIGN KEY (plugin_id)
REFERENCES zappstore.plugins (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: plugins_categories_category_id | type: CONSTRAINT --
-- ALTER TABLE zappstore.categories_plugins DROP CONSTRAINT IF EXISTS plugins_categories_category_id CASCADE;
ALTER TABLE zappstore.categories_plugins ADD CONSTRAINT plugins_categories_category_id FOREIGN KEY (category_id)
REFERENCES zappstore.categories (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: plugin_documentation_links_fk_plugin | type: CONSTRAINT --
-- ALTER TABLE zappstore.plugin_documentation_links DROP CONSTRAINT IF EXISTS plugin_documentation_links_fk_plugin CASCADE;
ALTER TABLE zappstore.plugin_documentation_links ADD CONSTRAINT plugin_documentation_links_fk_plugin FOREIGN KEY (plugin_id)
REFERENCES zappstore.plugins (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: cluster_events_fk_cluster | type: CONSTRAINT --
-- ALTER TABLE tenant.cluster_events DROP CONSTRAINT IF EXISTS cluster_events_fk_cluster CASCADE;
ALTER TABLE tenant.cluster_events ADD CONSTRAINT cluster_events_fk_cluster FOREIGN KEY (cluster_id)
REFERENCES tenant.clusters (id) MATCH SIMPLE
ON DELETE CASCADE ON UPDATE NO ACTION;
-- ddl-end --

-- object: "grant_U_ad521dc726" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA zappstore
   TO fun_fundament_api;

-- ddl-end --


-- object: "grant_U_fc33f17a39" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA tenant
   TO fun_fundament_api;

-- ddl-end --


-- object: "grant_U_a09934b29e" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA tenant
   TO fun_authn_api;

-- ddl-end --


-- object: grant_raw_6dafe3fd95 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.organizations
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_c16308945d | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.organizations
   TO fun_authn_api;

-- ddl-end --


-- object: grant_raw_b5e3aab1d0 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.projects
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_125a3754db | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.namespaces
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_d972e5c22a | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.users
   TO fun_authn_api;

-- ddl-end --


-- object: grant_raw_5940e5f705 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.users
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_940738ac34 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.clusters
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_31f2fce1d0 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.node_pools
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_2ca2d3950e | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.installs
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_b0f3fc5bb2 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.plugin_documentation_links
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_c7ef1230f0 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.plugins
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_bf1c10ddf6 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.categories_plugins
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_66c5b174fe | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.categories
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_638a3173d7 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.preset_plugins
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_00ef9ca13c | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.presets
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_71b6d05387 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.plugins_tags
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_raw_1585801963 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE zappstore.tags
   TO fun_fundament_api;

-- ddl-end --


-- object: "grant_U_94ccb226af" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA tenant
   TO fun_cluster_worker;

-- ddl-end --


-- object: grant_raw_8317ece277 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.clusters
   TO fun_cluster_worker;

-- ddl-end --


-- object: grant_raw_fcaa9ce53e | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.cluster_events
   TO fun_cluster_worker;

-- ddl-end --


-- object: grant_raw_eefd069f6a | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.cluster_events
   TO fun_fundament_api;

-- ddl-end --



