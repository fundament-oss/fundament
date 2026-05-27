SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "dcim"."device_catalogs" DROP CONSTRAINT "device_catalogs_ck_category";

ALTER TABLE "dcim"."device_catalogs" ADD CONSTRAINT "device_catalogs_ck_category" CHECK((category = ANY (ARRAY['server'::text, 'switch'::text, 'pdu'::text, 'patch_panel'::text, 'sfp'::text, 'nic'::text, 'cpu'::text, 'dimm'::text, 'disk'::text, 'cable'::text, 'adapter'::text, 'power_supply'::text, 'cable_manager'::text, 'console_server'::text, 'storage'::text, 'cooling'::text, 'firewall'::text, 'kvm'::text, 'gpu'::text, 'transceiver'::text, 'other'::text]))) NOT VALID;

ALTER TABLE "dcim"."device_catalogs" VALIDATE CONSTRAINT "device_catalogs_ck_category";

