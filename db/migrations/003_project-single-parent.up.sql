SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "tenant"."projects" ADD COLUMN "cluster_id" uuid NOT NULL;


CREATE OR REPLACE FUNCTION authn.is_project_in_organization(p_project_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.projects
    JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
    WHERE tenant.projects.id = p_project_id
    AND tenant.clusters.organization_id = authn.current_organization_id()
)
$function$
;

ALTER TABLE "tenant"."namespaces" DROP CONSTRAINT "namespaces_fk_cluster";

ALTER TABLE "tenant"."projects" DROP CONSTRAINT "projects_fk_organization";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
DROP INDEX "tenant"."namespaces_ix_cluster_name";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "namespaces_organization_policy" ON "tenant"."namespaces"
        USING (authn.is_project_in_organization(project_id));

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."namespaces" DROP COLUMN "cluster_id";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."projects" DROP CONSTRAINT "projects_uq_organization_name";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "projects_delete_policy" ON "tenant"."projects"
        USING ((authn.is_cluster_in_organization(cluster_id) AND authn.is_project_member(id, authn.current_user_id(), 'admin'::text)));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "projects_insert_policy" ON "tenant"."projects"
        WITH CHECK (authn.is_cluster_in_organization(cluster_id));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "projects_select_policy" ON "tenant"."projects"
        USING (authn.is_cluster_in_organization(cluster_id));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "projects_update_policy" ON "tenant"."projects"
        USING ((authn.is_cluster_in_organization(cluster_id) AND authn.is_project_member(id, authn.current_user_id(), 'admin'::text)));

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."projects" DROP COLUMN "organization_id";

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_fk_cluster";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX projects_uq_cluster_name ON tenant.projects USING btree (cluster_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_uq_cluster_name" UNIQUE USING INDEX "projects_uq_cluster_name";


/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.clusters_tr_verify_deleted()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM tenant.projects
		WHERE
			cluster_id = NEW.id
			AND deleted IS NULL
	) THEN
		RAISE EXCEPTION 'Cannot delete cluster with undeleted projects';
	END IF;
	RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.projects_tr_verify_deleted()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM tenant.namespaces
		WHERE
			project_id = NEW.id
			AND deleted IS NULL
	) THEN
		RAISE EXCEPTION 'Cannot delete project with undeleted namespaces';
	END IF;
	RETURN NEW;
END;
$function$
;

CREATE CONSTRAINT TRIGGER verify_deleted AFTER UPDATE ON tenant.projects NOT DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION tenant.projects_tr_verify_deleted();

CREATE INDEX namespaces_idx_project_id ON tenant.namespaces USING btree (project_id) WHERE (deleted IS NULL);
