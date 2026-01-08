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

-- object: authn | type: SCHEMA --
-- DROP SCHEMA IF EXISTS authn CASCADE;
CREATE SCHEMA authn;
-- ddl-end --
ALTER SCHEMA authn OWNER TO fun_owner;
-- ddl-end --

-- object: authz | type: SCHEMA --
-- DROP SCHEMA IF EXISTS authz CASCADE;
CREATE SCHEMA authz;
-- ddl-end --
ALTER SCHEMA authz OWNER TO fun_owner;
-- ddl-end --

SET search_path TO pg_catalog,public,tenant,zappstore,authn,authz;
-- ddl-end --

-- object: tenant.organizations | type: TABLE --
-- DROP TABLE IF EXISTS tenant.organizations CASCADE;
CREATE TABLE tenant.organizations (
	id uuid NOT NULL DEFAULT uuidv7(),
	name text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT organizations_pk PRIMARY KEY (id),
	CONSTRAINT organizations_uq_name UNIQUE NULLS NOT DISTINCT (name,deleted)
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

-- object: authn.current_user_id | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authn.current_user_id() CASCADE;
CREATE OR REPLACE FUNCTION authn.current_user_id ()
	RETURNS uuid
	LANGUAGE sql
	STABLE 
	CALLED ON NULL INPUT
	SECURITY INVOKER
	PARALLEL SAFE
	COST 1
	AS 
$function$
SELECT current_setting('app.current_user_id')::uuid
$function$;
-- ddl-end --
ALTER FUNCTION authn.current_user_id() OWNER TO fun_fundament_api;
-- ddl-end --

-- object: authn.current_organization_id | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authn.current_organization_id() CASCADE;
CREATE OR REPLACE FUNCTION authn.current_organization_id ()
	RETURNS uuid
	LANGUAGE sql
	STABLE 
	CALLED ON NULL INPUT
	SECURITY INVOKER
	PARALLEL SAFE
	COST 1
	AS 
$function$
SELECT current_setting('app.current_organization_id')::uuid
$function$;
-- ddl-end --
ALTER FUNCTION authn.current_organization_id() OWNER TO fun_fundament_api;
-- ddl-end --

-- object: authn.is_project_member | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authn.is_project_member(uuid,uuid,text) CASCADE;
CREATE OR REPLACE FUNCTION authn.is_project_member (IN p_project_id uuid, IN p_user_id uuid, IN p_role text)
	RETURNS boolean
	LANGUAGE sql
	STABLE 
	CALLED ON NULL INPUT
	SECURITY DEFINER
	PARALLEL SAFE
	COST 1
	AS 
$function$
SELECT EXISTS (
    SELECT 1 FROM tenant.project_members
    WHERE project_id = p_project_id
    AND user_id = p_user_id
    AND (p_role IS NULL OR role = p_role)
    AND deleted IS NULL
)
$function$;
-- ddl-end --
ALTER FUNCTION authn.is_project_member(uuid,uuid,text) OWNER TO fun_authz;
-- ddl-end --

-- object: authn.is_project_in_organization | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authn.is_project_in_organization(uuid) CASCADE;
CREATE OR REPLACE FUNCTION authn.is_project_in_organization (IN p_project_id uuid)
	RETURNS boolean
	LANGUAGE sql
	STABLE 
	CALLED ON NULL INPUT
	SECURITY DEFINER
	PARALLEL SAFE
	COST 1
	AS 
$function$
SELECT EXISTS (
    SELECT 1 FROM tenant.projects
    WHERE id = p_project_id
    AND organization_id = authn.current_organization_id()
)
$function$;
-- ddl-end --
ALTER FUNCTION authn.is_project_in_organization(uuid) OWNER TO fun_authz;
-- ddl-end --

-- object: authn.is_cluster_in_organization | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authn.is_cluster_in_organization(uuid) CASCADE;
CREATE OR REPLACE FUNCTION authn.is_cluster_in_organization (IN p_cluster_id uuid)
	RETURNS boolean
	LANGUAGE sql
	STABLE 
	CALLED ON NULL INPUT
	SECURITY DEFINER
	PARALLEL SAFE
	COST 1
	AS 
$function$
SELECT EXISTS (
    SELECT 1 FROM tenant.clusters
    WHERE id = p_cluster_id
    AND organization_id = authn.current_organization_id()
)
$function$;
-- ddl-end --
ALTER FUNCTION authn.is_cluster_in_organization(uuid) OWNER TO fun_authz;
-- ddl-end --

-- object: authn.is_user_in_organization | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authn.is_user_in_organization(uuid) CASCADE;
CREATE OR REPLACE FUNCTION authn.is_user_in_organization (IN p_user_id uuid)
	RETURNS boolean
	LANGUAGE sql
	STABLE 
	CALLED ON NULL INPUT
	SECURITY DEFINER
	PARALLEL SAFE
	COST 1
	AS 
$function$
SELECT EXISTS (
    SELECT 1 FROM tenant.users
    WHERE id = p_user_id
    AND organization_id = authn.current_organization_id()
)
$function$;
-- ddl-end --
ALTER FUNCTION authn.is_user_in_organization(uuid) OWNER TO fun_authz;
-- ddl-end --

-- object: tenant.project_has_members | type: FUNCTION --
-- DROP FUNCTION IF EXISTS tenant.project_has_members(uuid) CASCADE;
CREATE OR REPLACE FUNCTION tenant.project_has_members (IN p_project_id uuid)
	RETURNS boolean
	LANGUAGE sql
	STABLE 
	CALLED ON NULL INPUT
	SECURITY DEFINER
	PARALLEL SAFE
	COST 1
	AS 
$function$
SELECT EXISTS (
    SELECT 1 FROM tenant.project_members
    WHERE project_id = p_project_id
    AND deleted IS NULL
)
$function$;
-- ddl-end --
ALTER FUNCTION tenant.project_has_members(uuid) OWNER TO fun_authz;
-- ddl-end --

-- object: tenant.project_members_tr_protect_last_admin | type: FUNCTION --
-- DROP FUNCTION IF EXISTS tenant.project_members_tr_protect_last_admin() CASCADE;
CREATE OR REPLACE FUNCTION tenant.project_members_tr_protect_last_admin ()
	RETURNS trigger
	LANGUAGE plpgsql
	VOLATILE 
	CALLED ON NULL INPUT
	SECURITY INVOKER
	PARALLEL UNSAFE
	COST 1
	AS 
$function$
DECLARE
    admin_count integer;
BEGIN
    -- Only check if we're soft-deleting an admin or demoting an admin (UPDATE role from admin)
    IF OLD.role = 'admin' AND OLD.deleted IS NULL THEN
        -- Check if this is a soft delete (setting deleted) or role demotion
        IF (NEW.deleted IS NOT NULL) OR (NEW.role != 'admin') THEN
            SELECT COUNT(*) INTO admin_count
            FROM tenant.project_members
            WHERE project_id = OLD.project_id
            AND role = 'admin'
            AND id != OLD.id
            AND deleted IS NULL;

            IF admin_count = 0 THEN
                RAISE EXCEPTION 'Cannot remove or demote the last admin of a project'
                            USING HINT = 'project_contains_one_admin';
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$function$;
-- ddl-end --
ALTER FUNCTION tenant.project_members_tr_protect_last_admin() OWNER TO postgres;
-- ddl-end --

-- object: tenant.projects_tr_require_admin | type: FUNCTION --
-- DROP FUNCTION IF EXISTS tenant.projects_tr_require_admin() CASCADE;
CREATE OR REPLACE FUNCTION tenant.projects_tr_require_admin ()
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
    IF NOT EXISTS (
        SELECT 1 FROM tenant.project_members
        WHERE project_id = NEW.id
        AND role = 'admin'
        AND deleted IS NULL
    ) THEN
        RAISE EXCEPTION 'Project must have at least one admin';
    END IF;
    RETURN NEW;
END;
$function$;
-- ddl-end --
ALTER FUNCTION tenant.projects_tr_require_admin() OWNER TO postgres;
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

-- object: authn.api_keys | type: TABLE --
-- DROP TABLE IF EXISTS authn.api_keys CASCADE;
CREATE TABLE authn.api_keys (
	id uuid NOT NULL DEFAULT uuidv7(),
	organization_id uuid NOT NULL,
	user_id uuid NOT NULL,
	name text NOT NULL,
	token_hash bytea NOT NULL,
	token_prefix text NOT NULL,
	expires timestamptz,
	revoked timestamptz,
	last_used timestamptz,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT api_keys_pk PRIMARY KEY (id),
	CONSTRAINT api_keys_uq_token_hash UNIQUE (token_hash),
	CONSTRAINT api_keys_uq_name UNIQUE NULLS NOT DISTINCT (organization_id,name,deleted)
);
-- ddl-end --
ALTER TABLE authn.api_keys OWNER TO fun_owner;
-- ddl-end --
ALTER TABLE authn.api_keys ENABLE ROW LEVEL SECURITY;
-- ddl-end --

-- object: api_keys_organization_policy | type: POLICY --
-- DROP POLICY IF EXISTS api_keys_organization_policy ON authn.api_keys CASCADE;
CREATE POLICY api_keys_organization_policy ON authn.api_keys
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (organization_id = current_setting('app.current_organization_id')::uuid AND user_id = current_setting('app.current_user_id')::uuid);
-- ddl-end --

-- object: authn.api_key_get_by_hash | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authn.api_key_get_by_hash(bytea) CASCADE;
CREATE OR REPLACE FUNCTION authn.api_key_get_by_hash (IN p_token_hash bytea)
	RETURNS authn.api_keys
	LANGUAGE plpgsql
	VOLATILE 
	CALLED ON NULL INPUT
	SECURITY DEFINER
	PARALLEL UNSAFE
	COST 10
	AS 
$function$
DECLARE
	result authn.api_keys;
	key_record authn.api_keys;
BEGIN
	SELECT * INTO key_record FROM authn.api_keys WHERE token_hash = p_token_hash;

	IF NOT FOUND THEN
		RETURN NULL;
	END IF;

	IF key_record.deleted IS NOT NULL THEN
		RAISE EXCEPTION 'API key has been deleted' USING HINT = 'api_key_deleted';
	END IF;

	IF key_record.revoked IS NOT NULL THEN
		RAISE EXCEPTION 'API key has been revoked' USING HINT = 'api_key_revoked';
	END IF;

	IF key_record.expires IS NOT NULL AND key_record.expires <= NOW() THEN
		RAISE EXCEPTION 'API key has expired' USING HINT = 'api_key_expired';
	END IF;

	UPDATE authn.api_keys
	SET last_used = NOW()
	WHERE id = key_record.id
	RETURNING * INTO result;

	RETURN result;
END;
$function$;
-- ddl-end --
ALTER FUNCTION authn.api_key_get_by_hash(bytea) OWNER TO fun_owner;
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
	USING (organization_id = authn.current_organization_id());
-- ddl-end --

-- object: cluster_worker_all_access | type: POLICY --
-- DROP POLICY IF EXISTS cluster_worker_all_access ON tenant.clusters CASCADE;
CREATE POLICY cluster_worker_all_access ON tenant.clusters
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);
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
	USING (authn.is_cluster_in_organization(cluster_id));
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
	USING (authn.is_cluster_in_organization(cluster_id));
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

-- object: require_admin | type: TRIGGER --
-- require_admin ON tenant.projects CASCADE;
CREATE CONSTRAINT TRIGGER require_admin
	AFTER INSERT 
	ON tenant.projects
	DEFERRABLE INITIALLY DEFERRED
	FOR EACH ROW
	EXECUTE PROCEDURE tenant.projects_tr_require_admin();
-- ddl-end --

-- object: tenant.project_members | type: TABLE --
-- DROP TABLE IF EXISTS tenant.project_members CASCADE;
CREATE TABLE tenant.project_members (
	id uuid NOT NULL DEFAULT uuidv7(),
	project_id uuid NOT NULL,
	user_id uuid NOT NULL,
	role text NOT NULL,
	created timestamptz NOT NULL DEFAULT now(),
	deleted timestamptz,
	CONSTRAINT project_members_pk PRIMARY KEY (id),
	CONSTRAINT project_members_ck_role CHECK (role IN ('admin', 'viewer')),
	CONSTRAINT project_members_uq_project_user UNIQUE NULLS NOT DISTINCT (project_id,user_id,deleted)
);
-- ddl-end --
ALTER TABLE tenant.project_members OWNER TO fun_owner;
-- ddl-end --
ALTER TABLE tenant.project_members ENABLE ROW LEVEL SECURITY;
-- ddl-end --

-- object: project_members_idx_project_id | type: INDEX --
-- DROP INDEX IF EXISTS tenant.project_members_idx_project_id CASCADE;
CREATE INDEX project_members_idx_project_id ON tenant.project_members
USING btree
(
	project_id
);
-- ddl-end --

-- object: protect_last_admin | type: TRIGGER --
-- DROP TRIGGER IF EXISTS protect_last_admin ON tenant.project_members CASCADE;
CREATE OR REPLACE TRIGGER protect_last_admin
	BEFORE UPDATE
	ON tenant.project_members
	FOR EACH ROW
	EXECUTE PROCEDURE tenant.project_members_tr_protect_last_admin();
-- ddl-end --

-- object: project_members_select_policy | type: POLICY --
-- DROP POLICY IF EXISTS project_members_select_policy ON tenant.project_members CASCADE;
CREATE POLICY project_members_select_policy ON tenant.project_members
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING (authn.is_project_in_organization(project_id)
AND (authn.is_project_member(project_id, authn.current_user_id(), NULL)
	OR user_id = authn.current_user_id()));
-- ddl-end --

-- object: project_members_insert_policy | type: POLICY --
-- DROP POLICY IF EXISTS project_members_insert_policy ON tenant.project_members CASCADE;
CREATE POLICY project_members_insert_policy ON tenant.project_members
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK (authn.is_project_in_organization(project_id)
AND (deleted IS NOT NULL OR authn.is_user_in_organization(user_id))
AND (
    authn.is_project_member(project_id, authn.current_user_id(), 'admin')
    OR NOT tenant.project_has_members(project_id)
));
-- ddl-end --

-- object: project_members_update_policy | type: POLICY --
-- DROP POLICY IF EXISTS project_members_update_policy ON tenant.project_members CASCADE;
CREATE POLICY project_members_update_policy ON tenant.project_members
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING (authn.is_project_in_organization(project_id)
AND authn.is_project_member(project_id, authn.current_user_id(), 'admin'));
-- ddl-end --

-- object: projects_select_policy | type: POLICY --
-- DROP POLICY IF EXISTS projects_select_policy ON tenant.projects CASCADE;
CREATE POLICY projects_select_policy ON tenant.projects
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING (organization_id = authn.current_organization_id());
-- ddl-end --

-- object: projects_insert_policy | type: POLICY --
-- DROP POLICY IF EXISTS projects_insert_policy ON tenant.projects CASCADE;
CREATE POLICY projects_insert_policy ON tenant.projects
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK (organization_id = authn.current_organization_id());
-- ddl-end --

-- object: projects_update_policy | type: POLICY --
-- DROP POLICY IF EXISTS projects_update_policy ON tenant.projects CASCADE;
CREATE POLICY projects_update_policy ON tenant.projects
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING (organization_id = authn.current_organization_id()
AND authn.is_project_member(id, authn.current_user_id(), 'admin'));
-- ddl-end --

-- object: projects_delete_policy | type: POLICY --
-- DROP POLICY IF EXISTS projects_delete_policy ON tenant.projects CASCADE;
CREATE POLICY projects_delete_policy ON tenant.projects
	AS PERMISSIVE
	FOR DELETE
	TO fun_fundament_api
	USING (organization_id = authn.current_organization_id()
AND authn.is_project_member(id, authn.current_user_id(), 'admin'));
-- ddl-end --

-- object: namespaces_organization_policy | type: POLICY --
-- DROP POLICY IF EXISTS namespaces_organization_policy ON tenant.namespaces CASCADE;
CREATE POLICY namespaces_organization_policy ON tenant.namespaces
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (authn.is_cluster_in_organization(cluster_id));
-- ddl-end --

-- object: authz.outbox | type: TABLE --
-- DROP TABLE IF EXISTS authz.outbox CASCADE;
CREATE TABLE authz.outbox (
	id uuid NOT NULL DEFAULT uuidv7(),
	user_id uuid,
	project_id uuid,
	project_member_id uuid,
	cluster_id uuid,
	node_pool_id uuid,
	namespace_id uuid,
	api_key_id uuid,
	install_id uuid,
	created timestamptz NOT NULL DEFAULT now(),
	processed timestamptz,
	retries integer NOT NULL DEFAULT 0,
	failed timestamptz,
	CONSTRAINT outbox_pk PRIMARY KEY (id),
	CONSTRAINT outbox_ck_single_fk CHECK (num_nonnulls(
	user_id,
	project_id,
	project_member_id,
	cluster_id,
	node_pool_id,
	namespace_id,
	api_key_id,
	install_id
) = 1)
);
-- ddl-end --
ALTER TABLE authz.outbox OWNER TO fun_owner;
-- ddl-end --

-- object: outbox_idx_unprocessed | type: INDEX --
-- DROP INDEX IF EXISTS authz.outbox_idx_unprocessed CASCADE;
CREATE INDEX outbox_idx_unprocessed ON authz.outbox
USING btree
(
	created
)
WHERE (processed IS NULL);
-- ddl-end --

-- object: authz.users_sync_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.users_sync_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.users_sync_trigger ()
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
    INSERT INTO authz.outbox (user_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.users_sync_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: authz.projects_sync_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.projects_sync_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.projects_sync_trigger ()
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
    INSERT INTO authz.outbox (project_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.projects_sync_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: authz.project_members_sync_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.project_members_sync_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.project_members_sync_trigger ()
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
    INSERT INTO authz.outbox (project_member_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.project_members_sync_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: authz.clusters_sync_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.clusters_sync_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.clusters_sync_trigger ()
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
    INSERT INTO authz.outbox (cluster_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.clusters_sync_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: authz.node_pools_sync_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.node_pools_sync_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.node_pools_sync_trigger ()
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
    INSERT INTO authz.outbox (node_pool_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.node_pools_sync_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: authz.namespaces_sync_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.namespaces_sync_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.namespaces_sync_trigger ()
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
    INSERT INTO authz.outbox (namespace_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.namespaces_sync_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: authz.api_keys_sync_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.api_keys_sync_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.api_keys_sync_trigger ()
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
    INSERT INTO authz.outbox (api_key_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.api_keys_sync_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: authz.installs_sync_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.installs_sync_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.installs_sync_trigger ()
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
    INSERT INTO authz.outbox (install_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.installs_sync_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: authz.outbox_notify_trigger | type: FUNCTION --
-- DROP FUNCTION IF EXISTS authz.outbox_notify_trigger() CASCADE;
CREATE OR REPLACE FUNCTION authz.outbox_notify_trigger ()
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
    PERFORM pg_notify('authz_outbox', '');
    RETURN NEW;
END;
$function$;
-- ddl-end --
ALTER FUNCTION authz.outbox_notify_trigger() OWNER TO fun_owner;
-- ddl-end --

-- object: users_outbox | type: TRIGGER --
-- DROP TRIGGER IF EXISTS users_outbox ON tenant.users CASCADE;
CREATE OR REPLACE TRIGGER users_outbox
	AFTER INSERT OR UPDATE
	ON tenant.users
	FOR EACH ROW
	EXECUTE PROCEDURE authz.users_sync_trigger();
-- ddl-end --

-- object: project_members_outbox | type: TRIGGER --
-- DROP TRIGGER IF EXISTS project_members_outbox ON tenant.project_members CASCADE;
CREATE OR REPLACE TRIGGER project_members_outbox
	AFTER INSERT OR UPDATE
	ON tenant.project_members
	FOR EACH ROW
	EXECUTE PROCEDURE authz.project_members_sync_trigger();
-- ddl-end --

-- object: projects_outbox | type: TRIGGER --
-- DROP TRIGGER IF EXISTS projects_outbox ON tenant.projects CASCADE;
CREATE OR REPLACE TRIGGER projects_outbox
	AFTER INSERT OR UPDATE
	ON tenant.projects
	FOR EACH ROW
	EXECUTE PROCEDURE authz.projects_sync_trigger();
-- ddl-end --

-- object: clusters_outbox | type: TRIGGER --
-- DROP TRIGGER IF EXISTS clusters_outbox ON tenant.clusters CASCADE;
CREATE OR REPLACE TRIGGER clusters_outbox
	AFTER INSERT OR UPDATE
	ON tenant.clusters
	FOR EACH ROW
	EXECUTE PROCEDURE authz.clusters_sync_trigger();
-- ddl-end --

-- object: node_pools_outbox | type: TRIGGER --
-- DROP TRIGGER IF EXISTS node_pools_outbox ON tenant.node_pools CASCADE;
CREATE OR REPLACE TRIGGER node_pools_outbox
	AFTER INSERT OR UPDATE
	ON tenant.node_pools
	FOR EACH ROW
	EXECUTE PROCEDURE authz.node_pools_sync_trigger();
-- ddl-end --

-- object: namespaces_outbox | type: TRIGGER --
-- DROP TRIGGER IF EXISTS namespaces_outbox ON tenant.namespaces CASCADE;
CREATE OR REPLACE TRIGGER namespaces_outbox
	AFTER INSERT OR UPDATE
	ON tenant.namespaces
	FOR EACH ROW
	EXECUTE PROCEDURE authz.namespaces_sync_trigger();
-- ddl-end --

-- object: api_keys_outbox | type: TRIGGER --
-- DROP TRIGGER IF EXISTS api_keys_outbox ON authn.api_keys CASCADE;
CREATE OR REPLACE TRIGGER api_keys_outbox
	AFTER INSERT OR DELETE OR UPDATE
	ON authn.api_keys
	FOR EACH ROW
	EXECUTE PROCEDURE authz.api_keys_sync_trigger();
-- ddl-end --

-- object: installs_outbox | type: TRIGGER --
-- DROP TRIGGER IF EXISTS installs_outbox ON zappstore.installs CASCADE;
CREATE OR REPLACE TRIGGER installs_outbox
	AFTER INSERT OR UPDATE
	ON zappstore.installs
	FOR EACH ROW
	EXECUTE PROCEDURE authz.installs_sync_trigger();
-- ddl-end --

-- object: outbox_notify | type: TRIGGER --
-- DROP TRIGGER IF EXISTS outbox_notify ON authz.outbox CASCADE;
CREATE OR REPLACE TRIGGER outbox_notify
	AFTER INSERT 
	ON authz.outbox
	FOR EACH ROW
	EXECUTE PROCEDURE authz.outbox_notify_trigger();
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

-- object: api_keys_fk_organization | type: CONSTRAINT --
-- ALTER TABLE authn.api_keys DROP CONSTRAINT IF EXISTS api_keys_fk_organization CASCADE;
ALTER TABLE authn.api_keys ADD CONSTRAINT api_keys_fk_organization FOREIGN KEY (organization_id)
REFERENCES tenant.organizations (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: api_keys_fk_user | type: CONSTRAINT --
-- ALTER TABLE authn.api_keys DROP CONSTRAINT IF EXISTS api_keys_fk_user CASCADE;
ALTER TABLE authn.api_keys ADD CONSTRAINT api_keys_fk_user FOREIGN KEY (user_id)
REFERENCES tenant.users (id) MATCH SIMPLE
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

-- object: project_members_fk_project | type: CONSTRAINT --
-- ALTER TABLE tenant.project_members DROP CONSTRAINT IF EXISTS project_members_fk_project CASCADE;
ALTER TABLE tenant.project_members ADD CONSTRAINT project_members_fk_project FOREIGN KEY (project_id)
REFERENCES tenant.projects (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: project_members_fk_user | type: CONSTRAINT --
-- ALTER TABLE tenant.project_members DROP CONSTRAINT IF EXISTS project_members_fk_user CASCADE;
ALTER TABLE tenant.project_members ADD CONSTRAINT project_members_fk_user FOREIGN KEY (user_id)
REFERENCES tenant.users (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: outbox_fk_user | type: CONSTRAINT --
-- ALTER TABLE authz.outbox DROP CONSTRAINT IF EXISTS outbox_fk_user CASCADE;
ALTER TABLE authz.outbox ADD CONSTRAINT outbox_fk_user FOREIGN KEY (user_id)
REFERENCES tenant.users (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: outbox_fk_project | type: CONSTRAINT --
-- ALTER TABLE authz.outbox DROP CONSTRAINT IF EXISTS outbox_fk_project CASCADE;
ALTER TABLE authz.outbox ADD CONSTRAINT outbox_fk_project FOREIGN KEY (project_id)
REFERENCES tenant.projects (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: outbox_fk_project_member | type: CONSTRAINT --
-- ALTER TABLE authz.outbox DROP CONSTRAINT IF EXISTS outbox_fk_project_member CASCADE;
ALTER TABLE authz.outbox ADD CONSTRAINT outbox_fk_project_member FOREIGN KEY (project_member_id)
REFERENCES tenant.project_members (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: outbox_fk_cluster | type: CONSTRAINT --
-- ALTER TABLE authz.outbox DROP CONSTRAINT IF EXISTS outbox_fk_cluster CASCADE;
ALTER TABLE authz.outbox ADD CONSTRAINT outbox_fk_cluster FOREIGN KEY (cluster_id)
REFERENCES tenant.clusters (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: outbox_fk_node_pool | type: CONSTRAINT --
-- ALTER TABLE authz.outbox DROP CONSTRAINT IF EXISTS outbox_fk_node_pool CASCADE;
ALTER TABLE authz.outbox ADD CONSTRAINT outbox_fk_node_pool FOREIGN KEY (node_pool_id)
REFERENCES tenant.node_pools (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: outbox_fk_namespace | type: CONSTRAINT --
-- ALTER TABLE authz.outbox DROP CONSTRAINT IF EXISTS outbox_fk_namespace CASCADE;
ALTER TABLE authz.outbox ADD CONSTRAINT outbox_fk_namespace FOREIGN KEY (namespace_id)
REFERENCES tenant.namespaces (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: outbox_fk_api_key | type: CONSTRAINT --
-- ALTER TABLE authz.outbox DROP CONSTRAINT IF EXISTS outbox_fk_api_key CASCADE;
ALTER TABLE authz.outbox ADD CONSTRAINT outbox_fk_api_key FOREIGN KEY (api_key_id)
REFERENCES authn.api_keys (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
-- ddl-end --

-- object: outbox_fk_install | type: CONSTRAINT --
-- ALTER TABLE authz.outbox DROP CONSTRAINT IF EXISTS outbox_fk_install CASCADE;
ALTER TABLE authz.outbox ADD CONSTRAINT outbox_fk_install FOREIGN KEY (install_id)
REFERENCES zappstore.installs (id) MATCH SIMPLE
ON DELETE NO ACTION ON UPDATE NO ACTION;
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


-- object: "grant_U_3e9a923f30" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA authn
   TO fun_authn_api;

-- ddl-end --


-- object: grant_rw_a72779b347 | type: PERMISSION --
GRANT SELECT,UPDATE
   ON TABLE authn.api_keys
   TO fun_authn_api;

-- ddl-end --


-- object: grant_raw_3b85772350 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE authn.api_keys
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_r_b3ab3767d0 | type: PERMISSION --
GRANT SELECT
   ON TABLE tenant.projects
   TO fun_authz;

-- ddl-end --


-- object: grant_r_c9a2fb7fe9 | type: PERMISSION --
GRANT SELECT
   ON TABLE tenant.clusters
   TO fun_authz;

-- ddl-end --


-- object: grant_r_e2a5068826 | type: PERMISSION --
GRANT SELECT
   ON TABLE tenant.users
   TO fun_authz;

-- ddl-end --


-- object: grant_raw_f8d413afd5 | type: PERMISSION --
GRANT SELECT,INSERT,UPDATE
   ON TABLE tenant.project_members
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_r_84509e30ec | type: PERMISSION --
GRANT SELECT
   ON TABLE tenant.project_members
   TO fun_authz;

-- ddl-end --


-- object: "grant_U_30b8192bf7" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA tenant
   TO fun_authz;

-- ddl-end --


-- object: "grant_U_1f3f81f05c" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA authn
   TO fun_fundament_api;

-- ddl-end --


-- object: "grant_U_0c2c87179d" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA authn
   TO fun_authz;

-- ddl-end --


-- object: grant_a_9bf38d7215 | type: PERMISSION --
GRANT INSERT
   ON TABLE authz.outbox
   TO fun_fundament_api;

-- ddl-end --


-- object: grant_a_c5790f0447 | type: PERMISSION --
GRANT INSERT
   ON TABLE authz.outbox
   TO fun_authn_api;

-- ddl-end --


-- object: "grant_U_9d480d2da8" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA authz
   TO fun_fundament_api;

-- ddl-end --


-- object: "grant_U_1a39a09721" | type: PERMISSION --
GRANT USAGE
   ON SCHEMA authz
   TO fun_authn_api;

-- ddl-end --



