SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "dcim"."physical_connections" ADD COLUMN "cable_type" text COLLATE "pg_catalog"."default";

ALTER TABLE "dcim"."physical_connections" ADD CONSTRAINT "physical_connections_ck_cable_type" CHECK(((cable_type IS NULL) OR (cable_type = ANY (ARRAY['cat5e'::text, 'cat6'::text, 'cat6a'::text, 'cat7'::text, 'cat8'::text, 'dac'::text, 'aoc'::text, 'mmf'::text, 'smf'::text, 'power'::text, 'console'::text, 'usb'::text, 'other'::text])))) NOT VALID;

ALTER TABLE "dcim"."physical_connections" VALIDATE CONSTRAINT "physical_connections_ck_cable_type";

ALTER TABLE "dcim"."physical_connections" ADD COLUMN "color" text COLLATE "pg_catalog"."default";

ALTER TABLE "dcim"."physical_connections" ADD CONSTRAINT "physical_connections_ck_color" CHECK(((color IS NULL) OR (color = ANY (ARRAY['dark_grey'::text, 'light_grey'::text, 'red'::text, 'green'::text, 'blue'::text, 'yellow'::text, 'purple'::text, 'orange'::text, 'teal'::text, 'white'::text])))) NOT VALID;

ALTER TABLE "dcim"."physical_connections" VALIDATE CONSTRAINT "physical_connections_ck_color";

ALTER TABLE "dcim"."physical_connections" ADD COLUMN "label" text COLLATE "pg_catalog"."default";

ALTER TABLE "dcim"."physical_connections" ADD COLUMN "status" text COLLATE "pg_catalog"."default";

ALTER TABLE "dcim"."physical_connections" ADD CONSTRAINT "physical_connections_ck_status" CHECK(((status IS NULL) OR (status = ANY (ARRAY['planned'::text, 'connected'::text, 'decommissioned'::text])))) NOT VALID;

ALTER TABLE "dcim"."physical_connections" VALIDATE CONSTRAINT "physical_connections_ck_status";

