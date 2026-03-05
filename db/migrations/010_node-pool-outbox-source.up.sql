SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE tenant.cluster_outbox DROP CONSTRAINT cluster_outbox_ck_source;
ALTER TABLE tenant.cluster_outbox ADD CONSTRAINT cluster_outbox_ck_source CHECK (source IN ('trigger', 'reconcile', 'manual', 'node_pool'));

CREATE OR REPLACE FUNCTION tenant.node_pool_outbox_trigger()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
    VALUES (
        COALESCE(NEW.cluster_id, OLD.cluster_id),
        CASE
            WHEN TG_OP = 'INSERT' THEN 'created'
            WHEN OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN 'deleted'
            ELSE 'updated'
        END,
        'node_pool'
    );
    RETURN NULL;
END;
$$;
