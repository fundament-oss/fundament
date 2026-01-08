SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "tenant";


CREATE TABLE "tenant"."clusters" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"region" text COLLATE "pg_catalog"."default" NOT NULL,
	"kubernetes_version" text COLLATE "pg_catalog"."default" NOT NULL,
	"status" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_status" CHECK((status = ANY (ARRAY['unspecified'::text, 'provisioning'::text, 'starting'::text, 'running'::text, 'upgrading'::text, 'error'::text, 'stopping'::text, 'stopped'::text])));

CREATE POLICY "organization_isolation" ON "tenant"."clusters"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((organization_id = (current_setting('app.current_organization_id'::text))::uuid));

ALTER TABLE "tenant"."clusters" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX clusters_pk ON tenant.clusters USING btree (id);

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_pk" PRIMARY KEY USING INDEX "clusters_pk";

CREATE UNIQUE INDEX clusters_uq_name ON tenant.clusters USING btree (organization_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_uq_name" UNIQUE USING INDEX "clusters_uq_name";

CREATE OR REPLACE FUNCTION tenant.clusters_tr_verify_deleted()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
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
$function$
;

CREATE CONSTRAINT TRIGGER verify_deleted
AFTER INSERT OR UPDATE ON tenant.clusters
NOT DEFERRABLE INITIALLY IMMEDIATE
FOR EACH ROW
EXECUTE FUNCTION tenant.clusters_tr_verify_deleted();

CREATE TABLE "tenant"."namespaces" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"project_id" uuid NOT NULL,
	"cluster_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_ck_name" CHECK((name = name));

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."namespaces" VALIDATE CONSTRAINT "namespaces_fk_cluster";

CREATE UNIQUE INDEX namespaces_pk ON tenant.namespaces USING btree (id);

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_pk" PRIMARY KEY USING INDEX "namespaces_pk";

CREATE UNIQUE INDEX namespaces_uq_name ON tenant.namespaces USING btree (project_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_uq_name" UNIQUE USING INDEX "namespaces_uq_name";

CREATE TABLE "tenant"."organizations" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX organizations_pk ON tenant.organizations USING btree (id);

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_pk" PRIMARY KEY USING INDEX "organizations_pk";

CREATE UNIQUE INDEX organizations_uq_name ON tenant.organizations USING btree (name);

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_uq_name" UNIQUE USING INDEX "organizations_uq_name";

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_fk_organization";

CREATE TABLE "tenant"."projects" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_fk_organization";

CREATE UNIQUE INDEX projects_pk ON tenant.projects USING btree (id);

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_pk" PRIMARY KEY USING INDEX "projects_pk";

CREATE UNIQUE INDEX projects_uq_organization_name ON tenant.projects USING btree (organization_id, name);

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_uq_organization_name" UNIQUE USING INDEX "projects_uq_organization_name";

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "tenant"."namespaces" VALIDATE CONSTRAINT "namespaces_fk_project";

CREATE TABLE "tenant"."users" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"external_id" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."users" VALIDATE CONSTRAINT "users_fk_organization";

CREATE UNIQUE INDEX users_pk ON tenant.users USING btree (id);

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_pk" PRIMARY KEY USING INDEX "users_pk";

CREATE UNIQUE INDEX users_uq_external_id ON tenant.users USING btree (external_id);

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_uq_external_id" UNIQUE USING INDEX "users_uq_external_id";

-- Cluster sync state table (for cluster-worker to sync clusters to Gardener)
-- Create the cluster-worker role if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'fun_cluster_worker') THEN
        CREATE ROLE fun_cluster_worker;
    END IF;
END
$$;

CREATE TABLE tenant.cluster_sync (
    cluster_id uuid PRIMARY KEY REFERENCES tenant.clusters(id) ON DELETE CASCADE,
    synced timestamptz,
    sync_error text,
    sync_attempts int NOT NULL DEFAULT 0,
    sync_last_attempt timestamptz,
    shoot_status text,
    shoot_status_message text,
    shoot_status_updated timestamptz
);

COMMENT ON TABLE tenant.cluster_sync IS 'Sync state for clusters to Gardener. Separated from clusters table for cleaner separation of concerns.';

-- Auto-create sync row on cluster INSERT
CREATE OR REPLACE FUNCTION tenant.cluster_sync_create_on_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO tenant.cluster_sync (cluster_id)
    VALUES (NEW.id);
    RETURN NEW;
END;
$$;

CREATE TRIGGER cluster_sync_create
AFTER INSERT ON tenant.clusters
FOR EACH ROW
EXECUTE FUNCTION tenant.cluster_sync_create_on_insert();

-- Notify on sync needed
CREATE OR REPLACE FUNCTION tenant.cluster_sync_notify()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.synced IS NULL THEN
        PERFORM pg_notify('cluster_sync', NEW.cluster_id::text);
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER cluster_sync_notify
AFTER INSERT OR UPDATE OF synced
ON tenant.cluster_sync
FOR EACH ROW
EXECUTE FUNCTION tenant.cluster_sync_notify();

-- Reset synced when cluster is soft-deleted
CREATE OR REPLACE FUNCTION tenant.cluster_sync_reset_on_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
        UPDATE tenant.cluster_sync
        SET synced = NULL
        WHERE cluster_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER cluster_sync_reset_on_delete
AFTER UPDATE OF deleted ON tenant.clusters
FOR EACH ROW
EXECUTE FUNCTION tenant.cluster_sync_reset_on_delete();

-- Indexes for efficient polling
CREATE INDEX cluster_sync_idx_unsynced
ON tenant.cluster_sync (cluster_id)
WHERE synced IS NULL;

CREATE INDEX cluster_sync_idx_status_check
ON tenant.cluster_sync (shoot_status_updated)
WHERE synced IS NOT NULL;

-- Grant permissions to cluster-worker role
GRANT USAGE ON SCHEMA tenant TO fun_cluster_worker;
GRANT SELECT ON tenant.clusters TO fun_cluster_worker;
GRANT SELECT ON tenant.organizations TO fun_cluster_worker;
GRANT SELECT, INSERT, UPDATE ON tenant.cluster_sync TO fun_cluster_worker;
