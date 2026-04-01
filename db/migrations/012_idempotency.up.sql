SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "authz"."outbox" TO "fun_fundament_api";

CREATE TABLE "tenant"."idempotency_keys" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"idempotency_key" uuid NOT NULL,
	"user_id" uuid NOT NULL,
	"procedure" text COLLATE "pg_catalog"."default" NOT NULL,
	"request_hash" bytea NOT NULL,
	"response_bytes" bytea,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"expires" timestamp with time zone NOT NULL,
	"project_id" uuid,
	"project_member_id" uuid,
	"cluster_id" uuid,
	"node_pool_id" uuid,
	"namespace_id" uuid,
	"api_key_id" uuid,
	"install_id" uuid,
	"organization_user_id" uuid
);

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_ck_single_fk" CHECK((num_nonnulls(project_id, project_member_id, cluster_id, node_pool_id, namespace_id, api_key_id, install_id, organization_user_id) <= 1));

CREATE POLICY "idempotency_keys_cleanup_policy" ON "tenant"."idempotency_keys"
	AS PERMISSIVE
	FOR DELETE
	TO fun_fundament_api
	USING ((expires < now()));

CREATE POLICY "idempotency_keys_fundament_api_policy" ON "tenant"."idempotency_keys"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((user_id = authn.current_user_id()));

ALTER TABLE "tenant"."idempotency_keys" ENABLE ROW LEVEL SECURITY;

GRANT DELETE ON "tenant"."idempotency_keys" TO "fun_fundament_api";

GRANT INSERT ON "tenant"."idempotency_keys" TO "fun_fundament_api";

GRANT SELECT ON "tenant"."idempotency_keys" TO "fun_fundament_api";

GRANT UPDATE ON "tenant"."idempotency_keys" TO "fun_fundament_api";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_api_key" FOREIGN KEY (api_key_id) REFERENCES authn.api_keys(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_api_key";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_cluster";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_install" FOREIGN KEY (install_id) REFERENCES appstore.installs(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_install";

CREATE UNIQUE INDEX idempotency_keys_pk ON tenant.idempotency_keys USING btree (id);

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_pk" PRIMARY KEY USING INDEX "idempotency_keys_pk";

CREATE UNIQUE INDEX idempotency_keys_uq_key_user ON tenant.idempotency_keys USING btree (idempotency_key, user_id);

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_uq_key_user" UNIQUE USING INDEX "idempotency_keys_uq_key_user";

CREATE INDEX idempotency_keys_idx_expires ON tenant.idempotency_keys USING btree (expires);

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_namespace" FOREIGN KEY (namespace_id) REFERENCES tenant.namespaces(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_namespace";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_node_pool" FOREIGN KEY (node_pool_id) REFERENCES tenant.node_pools(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_node_pool";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_organization_user" FOREIGN KEY (organization_user_id) REFERENCES tenant.organizations_users(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_organization_user";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_project_member" FOREIGN KEY (project_member_id) REFERENCES tenant.project_members(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_project_member";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_project";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_fk_user";


-- Statements generated automatically, please review:
ALTER TABLE tenant.idempotency_keys OWNER TO fun_owner;
