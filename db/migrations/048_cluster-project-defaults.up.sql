SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.clusters_defaults_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    -- Reconcile the LimitRange of every active namespace on the cluster. No
    -- INSERT branch: a new cluster has no projects yet.
    IF NEW.deleted IS NULL
       AND (OLD.default_memory_request_mi IS DISTINCT FROM NEW.default_memory_request_mi
            OR OLD.default_memory_limit_mi IS DISTINCT FROM NEW.default_memory_limit_mi
            OR OLD.default_cpu_request_m IS DISTINCT FROM NEW.default_cpu_request_m
            OR OLD.default_cpu_limit_m IS DISTINCT FROM NEW.default_cpu_limit_m)
    THEN
        INSERT INTO tenant.cluster_outbox (namespace_id, event, source)
        SELECT tenant.namespaces.id, 'updated', 'trigger'
        FROM tenant.namespaces
        JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
        WHERE tenant.projects.cluster_id = NEW.id
          AND tenant.projects.deleted IS NULL
          AND tenant.namespaces.deleted IS NULL;
    END IF;

    RETURN NULL;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.clusters_tr_verify_defaults()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
DECLARE
    v_project tenant.projects;
    v_reason text;
BEGIN
    -- Lowering a cluster default below a project's is rejected rather than
    -- cascaded: the message names the project to lower first.
    FOR v_project IN
        SELECT *
        FROM tenant.projects
        WHERE tenant.projects.cluster_id = NEW.id
          AND tenant.projects.deleted IS NULL
        ORDER BY tenant.projects.name
    LOOP
        v_reason := tenant.project_defaults_violation(
            NEW.default_memory_request_mi, NEW.default_memory_limit_mi,
            NEW.default_cpu_request_m, NEW.default_cpu_limit_m,
            v_project.default_memory_request_mi, v_project.default_memory_limit_mi,
            v_project.default_cpu_request_m, v_project.default_cpu_limit_m);
        IF v_reason IS NOT NULL THEN
            RAISE EXCEPTION 'project %: %', v_project.name, v_reason
                        USING HINT = 'cluster_defaults_below_project';
        END IF;
    END LOOP;

    RETURN NULL;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.project_defaults_violation(p_cluster_memory_request_mi integer, p_cluster_memory_limit_mi integer, p_cluster_cpu_request_m integer, p_cluster_cpu_limit_m integer, p_project_memory_request_mi integer, p_project_memory_limit_mi integer, p_project_cpu_request_m integer, p_project_cpu_limit_m integer)
 RETURNS text
 LANGUAGE plpgsql
 IMMUTABLE PARALLEL SAFE COST 1
AS $function$
BEGIN
    -- A project default may never be higher than the cluster's, per field and
    -- only where both are set: an unset cluster default is no ceiling at all.
    -- The effective pair (the project's value, else the cluster's) must also
    -- stay a valid LimitRange, which a mix of set and unset values can break
    -- even though neither row violates its own limit >= request constraint.
    -- A comparison against NULL yields NULL, so every check below is skipped
    -- unless both of its sides are set.

    -- CPU, in millicores.
    IF p_project_cpu_request_m > p_cluster_cpu_request_m THEN
        RETURN format('default CPU request %sm exceeds the cluster''s %sm',
                      p_project_cpu_request_m, p_cluster_cpu_request_m);
    END IF;

    IF p_project_cpu_limit_m > p_cluster_cpu_limit_m THEN
        RETURN format('default CPU limit %sm exceeds the cluster''s %sm',
                      p_project_cpu_limit_m, p_cluster_cpu_limit_m);
    END IF;

    IF COALESCE(p_project_cpu_request_m, p_cluster_cpu_request_m)
       > COALESCE(p_project_cpu_limit_m, p_cluster_cpu_limit_m)
    THEN
        RETURN format('effective default CPU request %sm exceeds the effective limit %sm',
                      COALESCE(p_project_cpu_request_m, p_cluster_cpu_request_m),
                      COALESCE(p_project_cpu_limit_m, p_cluster_cpu_limit_m));
    END IF;

    -- Memory, in mebibytes.
    IF p_project_memory_request_mi > p_cluster_memory_request_mi THEN
        RETURN format('default memory request %sMi exceeds the cluster''s %sMi',
                      p_project_memory_request_mi, p_cluster_memory_request_mi);
    END IF;

    IF p_project_memory_limit_mi > p_cluster_memory_limit_mi THEN
        RETURN format('default memory limit %sMi exceeds the cluster''s %sMi',
                      p_project_memory_limit_mi, p_cluster_memory_limit_mi);
    END IF;

    IF COALESCE(p_project_memory_request_mi, p_cluster_memory_request_mi)
       > COALESCE(p_project_memory_limit_mi, p_cluster_memory_limit_mi)
    THEN
        RETURN format('effective default memory request %sMi exceeds the effective limit %sMi',
                      COALESCE(p_project_memory_request_mi, p_cluster_memory_request_mi),
                      COALESCE(p_project_memory_limit_mi, p_cluster_memory_limit_mi));
    END IF;

    RETURN NULL;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.projects_defaults_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    -- Reconcile the LimitRange of the project's active namespaces. No INSERT
    -- branch: a new project has no namespaces yet.
    IF NEW.deleted IS NULL
       AND (OLD.default_memory_request_mi IS DISTINCT FROM NEW.default_memory_request_mi
            OR OLD.default_memory_limit_mi IS DISTINCT FROM NEW.default_memory_limit_mi
            OR OLD.default_cpu_request_m IS DISTINCT FROM NEW.default_cpu_request_m
            OR OLD.default_cpu_limit_m IS DISTINCT FROM NEW.default_cpu_limit_m)
    THEN
        INSERT INTO tenant.cluster_outbox (namespace_id, event, source)
        SELECT tenant.namespaces.id, 'updated', 'trigger'
        FROM tenant.namespaces
        WHERE tenant.namespaces.project_id = NEW.id
          AND tenant.namespaces.deleted IS NULL;
    END IF;

    RETURN NULL;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.projects_tr_verify_defaults()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
DECLARE
    v_cluster tenant.clusters;
    v_reason text;
BEGIN
    IF NEW.deleted IS NOT NULL THEN
        RETURN NULL;
    END IF;

    -- FOR SHARE, not a plain read: a cluster lowering its defaults holds the
    -- lock on its row, so under READ COMMITTED this check waits for it instead
    -- of passing against values that transaction is about to replace. The
    -- trigger body is volatile, so its query takes a fresh snapshot and sees
    -- whichever transaction committed first.
    SELECT * INTO v_cluster
    FROM tenant.clusters
    WHERE tenant.clusters.id = NEW.cluster_id
    FOR SHARE;

    v_reason := tenant.project_defaults_violation(
        v_cluster.default_memory_request_mi, v_cluster.default_memory_limit_mi,
        v_cluster.default_cpu_request_m, v_cluster.default_cpu_limit_m,
        NEW.default_memory_request_mi, NEW.default_memory_limit_mi,
        NEW.default_cpu_request_m, NEW.default_cpu_limit_m);
    IF v_reason IS NOT NULL THEN
        RAISE EXCEPTION '%', v_reason
                    USING HINT = 'project_defaults_exceed_cluster';
    END IF;

    RETURN NULL;
END;
$function$
;

ALTER TABLE "tenant"."clusters" ADD COLUMN "default_cpu_limit_m" integer;

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_default_cpu_limit_m" CHECK(((default_cpu_limit_m IS NULL) OR (default_cpu_limit_m > 0))) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_ck_default_cpu_limit_m";

ALTER TABLE "tenant"."clusters" ADD COLUMN "default_cpu_request_m" integer;

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_default_cpu_limit_gte_request" CHECK(((default_cpu_limit_m IS NULL) OR (default_cpu_request_m IS NULL) OR (default_cpu_limit_m >= default_cpu_request_m))) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_ck_default_cpu_limit_gte_request";

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_default_cpu_request_m" CHECK(((default_cpu_request_m IS NULL) OR (default_cpu_request_m > 0))) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_ck_default_cpu_request_m";

ALTER TABLE "tenant"."clusters" ADD COLUMN "default_memory_limit_mi" integer;

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_default_memory_limit_mi" CHECK(((default_memory_limit_mi IS NULL) OR (default_memory_limit_mi > 0))) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_ck_default_memory_limit_mi";

ALTER TABLE "tenant"."clusters" ADD COLUMN "default_memory_request_mi" integer;

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_default_memory_limit_gte_request" CHECK(((default_memory_limit_mi IS NULL) OR (default_memory_request_mi IS NULL) OR (default_memory_limit_mi >= default_memory_request_mi))) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_ck_default_memory_limit_gte_request";

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_default_memory_request_mi" CHECK(((default_memory_request_mi IS NULL) OR (default_memory_request_mi > 0))) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_ck_default_memory_request_mi";

CREATE TRIGGER defaults_outbox AFTER UPDATE OF default_memory_request_mi, default_memory_limit_mi, default_cpu_request_m, default_cpu_limit_m ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION tenant.clusters_defaults_outbox_trigger();

CREATE CONSTRAINT TRIGGER verify_defaults AFTER UPDATE OF default_memory_request_mi, default_memory_limit_mi, default_cpu_request_m, default_cpu_limit_m ON tenant.clusters NOT DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION tenant.clusters_tr_verify_defaults();

ALTER TABLE "tenant"."organization_limits" DROP CONSTRAINT "organization_limits_fk_organization";

ALTER TABLE "tenant"."project_limits" DROP CONSTRAINT "project_limits_fk_project";

ALTER TABLE "tenant"."projects" ADD COLUMN "default_cpu_limit_m" integer;

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_ck_default_cpu_limit_m" CHECK(((default_cpu_limit_m IS NULL) OR (default_cpu_limit_m > 0))) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_ck_default_cpu_limit_m";

ALTER TABLE "tenant"."projects" ADD COLUMN "default_cpu_request_m" integer;

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_ck_default_cpu_limit_gte_request" CHECK(((default_cpu_limit_m IS NULL) OR (default_cpu_request_m IS NULL) OR (default_cpu_limit_m >= default_cpu_request_m))) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_ck_default_cpu_limit_gte_request";

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_ck_default_cpu_request_m" CHECK(((default_cpu_request_m IS NULL) OR (default_cpu_request_m > 0))) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_ck_default_cpu_request_m";

ALTER TABLE "tenant"."projects" ADD COLUMN "default_memory_limit_mi" integer;

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_ck_default_memory_limit_mi" CHECK(((default_memory_limit_mi IS NULL) OR (default_memory_limit_mi > 0))) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_ck_default_memory_limit_mi";

ALTER TABLE "tenant"."projects" ADD COLUMN "default_memory_request_mi" integer;

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_ck_default_memory_limit_gte_request" CHECK(((default_memory_limit_mi IS NULL) OR (default_memory_request_mi IS NULL) OR (default_memory_limit_mi >= default_memory_request_mi))) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_ck_default_memory_limit_gte_request";

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_ck_default_memory_request_mi" CHECK(((default_memory_request_mi IS NULL) OR (default_memory_request_mi > 0))) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_ck_default_memory_request_mi";

CREATE TRIGGER defaults_outbox AFTER UPDATE OF default_memory_request_mi, default_memory_limit_mi, default_cpu_request_m, default_cpu_limit_m ON tenant.projects FOR EACH ROW EXECUTE FUNCTION tenant.projects_defaults_outbox_trigger();

CREATE CONSTRAINT TRIGGER verify_defaults AFTER INSERT OR UPDATE OF cluster_id, default_memory_request_mi, default_memory_limit_mi, default_cpu_request_m, default_cpu_limit_m ON tenant.projects NOT DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION tenant.projects_tr_verify_defaults();

-- Data: carry today's effective values over before the old tables go.
-- Every active cluster takes its organization's defaults.
UPDATE tenant.clusters
SET default_memory_request_mi = tenant.organization_limits.default_memory_request_mi,
    default_memory_limit_mi   = tenant.organization_limits.default_memory_limit_mi,
    default_cpu_request_m     = tenant.organization_limits.default_cpu_request_m,
    default_cpu_limit_m       = tenant.organization_limits.default_cpu_limit_m
FROM tenant.organization_limits
WHERE tenant.organization_limits.organization_id = tenant.clusters.organization_id
  AND tenant.organization_limits.deleted IS NULL
  AND tenant.clusters.deleted IS NULL;

-- Projects keep their own values, capped at the cluster's. Today the lower value
-- already wins, so effective values do not change. Unset stays unset: LEAST
-- ignores NULL, so an unset cluster value leaves the project value as is.
UPDATE tenant.projects
SET default_memory_request_mi = CASE WHEN tenant.project_limits.default_memory_request_mi IS NULL THEN NULL
        ELSE LEAST(tenant.project_limits.default_memory_request_mi, tenant.clusters.default_memory_request_mi) END,
    default_memory_limit_mi   = CASE WHEN tenant.project_limits.default_memory_limit_mi IS NULL THEN NULL
        ELSE LEAST(tenant.project_limits.default_memory_limit_mi, tenant.clusters.default_memory_limit_mi) END,
    default_cpu_request_m     = CASE WHEN tenant.project_limits.default_cpu_request_m IS NULL THEN NULL
        ELSE LEAST(tenant.project_limits.default_cpu_request_m, tenant.clusters.default_cpu_request_m) END,
    default_cpu_limit_m       = CASE WHEN tenant.project_limits.default_cpu_limit_m IS NULL THEN NULL
        ELSE LEAST(tenant.project_limits.default_cpu_limit_m, tenant.clusters.default_cpu_limit_m) END
FROM tenant.project_limits, tenant.clusters
WHERE tenant.project_limits.project_id = tenant.projects.id
  AND tenant.project_limits.deleted IS NULL
  AND tenant.clusters.id = tenant.projects.cluster_id
  AND tenant.projects.deleted IS NULL;

DROP TRIGGER "organization_limits_outbox" ON "tenant"."organization_limits";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "tenant"."organization_limits_outbox_trigger"();

SET SESSION statement_timeout = 1200000;

/* Hazards:
 - DELETES_DATA: Deletes all rows in the table (and the table itself)
*/
DROP TABLE "tenant"."organization_limits";

SET SESSION statement_timeout = 3000;

DROP TRIGGER "project_limits_outbox" ON "tenant"."project_limits";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "tenant"."project_limits_outbox_trigger"();

SET SESSION statement_timeout = 1200000;

/* Hazards:
 - DELETES_DATA: Deletes all rows in the table (and the table itself)
*/
DROP TABLE "tenant"."project_limits";


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.clusters_defaults_outbox_trigger() OWNER TO fun_owner;
ALTER FUNCTION tenant.clusters_tr_verify_defaults() OWNER TO fun_owner;
ALTER FUNCTION tenant.project_defaults_violation(p_cluster_memory_request_mi integer, p_cluster_memory_limit_mi integer, p_cluster_cpu_request_m integer, p_cluster_cpu_limit_m integer, p_project_memory_request_mi integer, p_project_memory_limit_mi integer, p_project_cpu_request_m integer, p_project_cpu_limit_m integer) OWNER TO fun_owner;
ALTER FUNCTION tenant.projects_defaults_outbox_trigger() OWNER TO fun_owner;
ALTER FUNCTION tenant.projects_tr_verify_defaults() OWNER TO fun_owner;
