SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

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
	RETURN NEW;
END;
$function$
;

ALTER TABLE "organization"."clusters" DROP CONSTRAINT "clusters_ck_status";

ALTER TABLE "organization"."clusters" ADD CONSTRAINT "clusters_ck_status" CHECK((status = ANY (ARRAY['unspecified'::text, 'provisioning'::text, 'starting'::text, 'running'::text, 'upgrading'::text, 'error'::text, 'stopping'::text, 'stopped'::text]))) NOT VALID;

ALTER TABLE "organization"."clusters" VALIDATE CONSTRAINT "clusters_ck_status";
