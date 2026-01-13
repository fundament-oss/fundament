SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX categories_plugins_pk ON zappstore.categories_plugins USING btree (plugin_id, category_id);

ALTER TABLE "zappstore"."categories_plugins" ADD CONSTRAINT "categories_plugins_pk" PRIMARY KEY USING INDEX "categories_plugins_pk";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX plugins_tags_pk ON zappstore.plugins_tags USING btree (plugin_id, tag_id);

ALTER TABLE "zappstore"."plugins_tags" ADD CONSTRAINT "plugins_tags_pk" PRIMARY KEY USING INDEX "plugins_tags_pk";

CREATE TABLE "zappstore"."preset_plugins" (
	"preset_id" uuid NOT NULL,
	"plugin_id" uuid NOT NULL
);

ALTER TABLE "zappstore"."preset_plugins" ADD CONSTRAINT "plugins_presets_plugin_id" FOREIGN KEY (plugin_id) REFERENCES zappstore.plugins(id) NOT VALID;

ALTER TABLE "zappstore"."preset_plugins" VALIDATE CONSTRAINT "plugins_presets_plugin_id";

CREATE UNIQUE INDEX preset_plugins_pk ON zappstore.preset_plugins USING btree (preset_id, plugin_id);

ALTER TABLE "zappstore"."preset_plugins" ADD CONSTRAINT "preset_plugins_pk" PRIMARY KEY USING INDEX "preset_plugins_pk";

CREATE TABLE "zappstore"."presets" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"description" text COLLATE "pg_catalog"."default"
);

CREATE UNIQUE INDEX presets_pk ON zappstore.presets USING btree (id);

ALTER TABLE "zappstore"."presets" ADD CONSTRAINT "presets_pk" PRIMARY KEY USING INDEX "presets_pk";

CREATE UNIQUE INDEX presets_uq_name ON zappstore.presets USING btree (name);

ALTER TABLE "zappstore"."presets" ADD CONSTRAINT "presets_uq_name" UNIQUE USING INDEX "presets_uq_name";

ALTER TABLE "zappstore"."preset_plugins" ADD CONSTRAINT "plugins_presets_preset_id" FOREIGN KEY (preset_id) REFERENCES zappstore.presets(id) NOT VALID;

ALTER TABLE "zappstore"."preset_plugins" VALIDATE CONSTRAINT "plugins_presets_preset_id";


-- Statements generated automatically, please review:
ALTER TABLE zappstore.preset_plugins OWNER TO fun_fundament_api;
ALTER TABLE zappstore.presets OWNER TO fun_fundament_api;
