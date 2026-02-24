SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_cluster_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF TG_OP = 'INSERT'
       OR OLD.deleted IS DISTINCT FROM NEW.deleted
       OR OLD.name IS DISTINCT FROM NEW.name
       OR OLD.region IS DISTINCT FROM NEW.region
       OR OLD.kubernetes_version IS DISTINCT FROM NEW.kubernetes_version
    THEN
        INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
        VALUES (COALESCE(NEW.id, OLD.id),
                CASE
                    WHEN TG_OP = 'INSERT' THEN 'created'
                    WHEN OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN 'deleted'
                    ELSE 'updated'
                END,
                'trigger');
    END IF;
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_notify()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    PERFORM pg_notify('cluster_outbox', '');
    RETURN NEW;
END;
$function$
;

CREATE TABLE "tenant"."cluster_outbox" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid,
	"namespace_id" uuid,
	"project_member_id" uuid,
	"project_id" uuid,
	"event" text COLLATE "pg_catalog"."default" DEFAULT 'updated'::text NOT NULL,
	"source" text COLLATE "pg_catalog"."default" DEFAULT 'trigger'::text NOT NULL,
	"status" text COLLATE "pg_catalog"."default" DEFAULT 'pending'::text NOT NULL,
	"retries" integer DEFAULT 0 NOT NULL,
	"retry_after" timestamp with time zone,
	"processed" timestamp with time zone,
	"failed" timestamp with time zone,
	"status_info" text COLLATE "pg_catalog"."default",
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_event" CHECK((event = ANY (ARRAY['created'::text, 'updated'::text, 'deleted'::text, 'reconcile'::text])));

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_single_fk" CHECK((num_nonnulls(cluster_id, namespace_id, project_member_id, project_id) = 1));

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_source" CHECK((source = ANY (ARRAY['trigger'::text, 'reconcile'::text, 'manual'::text])));

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_status" CHECK((status = ANY (ARRAY['pending'::text, 'completed'::text, 'retrying'::text, 'failed'::text])));

GRANT INSERT ON "tenant"."cluster_outbox" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."cluster_outbox" TO "fun_cluster_worker";

GRANT UPDATE ON "tenant"."cluster_outbox" TO "fun_cluster_worker";

GRANT INSERT ON "tenant"."cluster_outbox" TO "fun_fundament_api";

CREATE UNIQUE INDEX cluster_outbox_pk ON tenant.cluster_outbox USING btree (id);

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_pk" PRIMARY KEY USING INDEX "cluster_outbox_pk";

CREATE INDEX cluster_outbox_idx_cluster_id ON tenant.cluster_outbox USING btree (cluster_id);

CREATE INDEX cluster_outbox_idx_namespace_id ON tenant.cluster_outbox USING btree (namespace_id);

CREATE INDEX cluster_outbox_idx_project_id ON tenant.cluster_outbox USING btree (project_id);

CREATE INDEX cluster_outbox_idx_project_member_id ON tenant.cluster_outbox USING btree (project_member_id);

CREATE INDEX cluster_outbox_status_retry_idx ON tenant.cluster_outbox USING btree (status, retry_after, id);

CREATE TRIGGER cluster_outbox_notify AFTER INSERT ON tenant.cluster_outbox FOR EACH ROW EXECUTE FUNCTION tenant.cluster_outbox_notify();

CREATE TRIGGER cluster_outbox_cluster AFTER INSERT OR UPDATE ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION tenant.cluster_outbox_cluster_trigger();

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_cluster";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_namespace" FOREIGN KEY (namespace_id) REFERENCES tenant.namespaces(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_namespace";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_cluster_worker_policy" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_members_cluster_worker_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_members" TO "fun_cluster_worker";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_project_member" FOREIGN KEY (project_member_id) REFERENCES tenant.project_members(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_project_member";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_cluster_worker_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_cluster_worker";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_project";


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.cluster_outbox_cluster_trigger() OWNER TO fun_owner;
ALTER FUNCTION tenant.cluster_outbox_notify() OWNER TO fun_owner;
ALTER TABLE tenant.cluster_outbox OWNER TO fun_owner;
