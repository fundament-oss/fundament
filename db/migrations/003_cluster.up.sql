create type "organization"."ClusterStatus" as enum ('unspecified', 'provisioning', 'starting', 'running', 'upgrading', 'error', 'stopping', 'stopped');

alter table "organization"."clusters" add column "kubernetes_version" text not null;

alter table "organization"."clusters" add column "region" text not null;

alter table "organization"."clusters" add column "status" organization."ClusterStatus" not null;

alter table "organization"."clusters" enable row level security;

alter table "organization"."clusters" add constraint "clusters_kubernetes_version_not_null" NOT NULL kubernetes_version;

alter table "organization"."clusters" add constraint "clusters_region_not_null" NOT NULL region;

alter table "organization"."clusters" add constraint "clusters_status_not_null" NOT NULL status;

create policy "tenant_isolation"
on "organization"."clusters"
as permissive
for all
to fun_organization_api
using ((tenant_id = (current_setting('app.current_tenant_id'::text))::uuid));

set check_function_bodies = off;

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