SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Status says how far a cable is towards existing. Each value names what has to
-- happen next.
--
-- 'planned' named none of them. It only said the cable was not there yet, so
-- "have we bought this" had nowhere to live and the same list came up for
-- ordering week after week. It is renamed to 'to_order', and two states are
-- added behind it: 'ordered' for what is on its way, and 'ready_to_install' for
-- what has arrived and is waiting for hands. That last one was missing
-- altogether, which put a cable on a lorry and a cable lying in the rack in the
-- same state.
--
-- Every existing row moves from 'planned' to 'to_order'. That is the earliest of
-- the three and the only one that claims nothing on somebody's behalf: this
-- database never recorded whether anything had been bought, so it cannot say so
-- now.
--
-- A cable that is drawn but not decided on keeps no status at all (NULL), which
-- is what it already meant.
--
-- Not reversible: 'ordered' and 'ready_to_install' cannot be folded back into
-- 'planned' without losing which of the three a row was in.

ALTER TABLE "dcim"."physical_connections" DROP CONSTRAINT "physical_connections_ck_status";

UPDATE "dcim"."physical_connections" SET "status" = 'to_order' WHERE "status" = 'planned';

ALTER TABLE "dcim"."physical_connections" ADD CONSTRAINT "physical_connections_ck_status" CHECK(((status IS NULL) OR (status = ANY (ARRAY['to_order'::text, 'ordered'::text, 'ready_to_install'::text, 'connected'::text, 'decommissioned'::text]))));
