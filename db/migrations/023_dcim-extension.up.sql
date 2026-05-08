SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."asset_events" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."asset_events" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."asset_events" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."assets" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."assets" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."assets" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."device_catalogs" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."device_catalogs" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."device_catalogs" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."logical_connections" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."logical_connections" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."logical_connections" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."logical_designs" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."logical_designs" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."logical_designs" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT DELETE ON "dcim"."logical_device_layouts" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."logical_device_layouts" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."logical_device_layouts" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."logical_device_layouts" TO "fun_dcim_api";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX logical_device_layouts_uq_device ON dcim.logical_device_layouts USING btree (logical_device_id);

ALTER TABLE "dcim"."logical_device_layouts" ADD CONSTRAINT "logical_device_layouts_uq_device" UNIQUE USING INDEX "logical_device_layouts_uq_device";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."logical_devices" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."logical_devices" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."logical_devices" TO "fun_dcim_api";

ALTER TABLE "dcim"."notes" ADD COLUMN "task_id" uuid;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."notes" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."notes" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."notes" TO "fun_dcim_api";

ALTER TABLE "dcim"."notes" DROP CONSTRAINT "notes_ck_single_ref";

ALTER TABLE "dcim"."notes" ADD CONSTRAINT "notes_ck_single_ref" CHECK((num_nonnulls(device_catalog_id, port_definition_id, asset_id, site_id, room_id, rack_row_id, rack_id, placement_id, physical_connection_id, logical_design_id, logical_device_id, logical_connection_id, task_id) = 1)) NOT VALID;

ALTER TABLE "dcim"."notes" VALIDATE CONSTRAINT "notes_ck_single_ref";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."physical_connections" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."physical_connections" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."physical_connections" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."placements" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."placements" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."placements" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."port_compatibilities" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."port_compatibilities" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."port_compatibilities" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."port_definitions" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."port_definitions" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."port_definitions" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."rack_rows" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."rack_rows" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."rack_rows" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."racks" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."racks" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."racks" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."rooms" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."rooms" TO "fun_dcim_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."rooms" TO "fun_dcim_api";

CREATE TABLE "dcim"."task_steps" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"task_id" uuid NOT NULL,
	"title" text COLLATE "pg_catalog"."default" NOT NULL,
	"description" text COLLATE "pg_catalog"."default",
	"ordinal" integer NOT NULL,
	"completed" boolean DEFAULT false NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

GRANT INSERT ON "dcim"."task_steps" TO "fun_dcim_api";

GRANT SELECT ON "dcim"."task_steps" TO "fun_dcim_api";

GRANT UPDATE ON "dcim"."task_steps" TO "fun_dcim_api";

CREATE UNIQUE INDEX task_steps_pk ON dcim.task_steps USING btree (id);

ALTER TABLE "dcim"."task_steps" ADD CONSTRAINT "task_steps_pk" PRIMARY KEY USING INDEX "task_steps_pk";

CREATE TABLE "dcim"."tasks" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"title" text COLLATE "pg_catalog"."default" NOT NULL,
	"description" text COLLATE "pg_catalog"."default",
	"status" text COLLATE "pg_catalog"."default" DEFAULT 'ready'::text NOT NULL,
	"priority" text COLLATE "pg_catalog"."default" DEFAULT 'medium'::text NOT NULL,
	"category" text COLLATE "pg_catalog"."default" DEFAULT 'other'::text NOT NULL,
	"assignee_id" text COLLATE "pg_catalog"."default",
	"due_date" timestamp with time zone,
	"location" text COLLATE "pg_catalog"."default",
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "dcim"."tasks" ADD CONSTRAINT "tasks_ck_category" CHECK((category = ANY (ARRAY['hardware'::text, 'network'::text, 'cooling'::text, 'power'::text, 'security'::text, 'other'::text])));

ALTER TABLE "dcim"."tasks" ADD CONSTRAINT "tasks_ck_priority" CHECK((priority = ANY (ARRAY['low'::text, 'medium'::text, 'high'::text, 'critical'::text])));

ALTER TABLE "dcim"."tasks" ADD CONSTRAINT "tasks_ck_status" CHECK((status = ANY (ARRAY['ready'::text, 'in_progress'::text, 'review'::text, 'blocked'::text, 'done'::text])));

GRANT INSERT ON "dcim"."tasks" TO "fun_dcim_api";

GRANT SELECT ON "dcim"."tasks" TO "fun_dcim_api";

GRANT UPDATE ON "dcim"."tasks" TO "fun_dcim_api";

CREATE UNIQUE INDEX tasks_pk ON dcim.tasks USING btree (id);

ALTER TABLE "dcim"."tasks" ADD CONSTRAINT "tasks_pk" PRIMARY KEY USING INDEX "tasks_pk";

ALTER TABLE "dcim"."notes" ADD CONSTRAINT "dcim_notes_fk_task" FOREIGN KEY (task_id) REFERENCES dcim.tasks(id) NOT VALID;

ALTER TABLE "dcim"."notes" VALIDATE CONSTRAINT "dcim_notes_fk_task";

ALTER TABLE "dcim"."task_steps" ADD CONSTRAINT "dcim_task_steps_fk_task" FOREIGN KEY (task_id) REFERENCES dcim.tasks(id) NOT VALID;

ALTER TABLE "dcim"."task_steps" VALIDATE CONSTRAINT "dcim_task_steps_fk_task";


-- Statements generated automatically, please review:
ALTER TABLE dcim.task_steps OWNER TO fun_owner;
ALTER TABLE dcim.tasks OWNER TO fun_owner;
