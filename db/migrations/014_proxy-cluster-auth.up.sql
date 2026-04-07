SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "shoot_api_server_url";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "shoot_ca_data";

