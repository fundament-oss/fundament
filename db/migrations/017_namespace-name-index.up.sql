SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX namespaces_ix_cluster_name ON tenant.namespaces USING btree (cluster_id, name) WHERE (deleted IS NULL);
