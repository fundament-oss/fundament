-- Separate table for cluster sync state.
-- This provides better separation of concerns and enables future multi-target sync.

-- Create the cluster-worker role if it doesn't exist (not subject to RLS on clusters table)
-- Note: Role may already exist if created by CloudNativePG managed roles
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'fun_cluster_worker') THEN
        CREATE ROLE fun_cluster_worker;
    END IF;
END
$$;

-- Create the cluster_sync table
CREATE TABLE organization.cluster_sync (
    cluster_id uuid PRIMARY KEY REFERENCES organization.clusters(id) ON DELETE CASCADE,

    -- Sync state
    synced timestamptz,                    -- NULL = needs sync, timestamp = last successful sync
    sync_error text,                       -- Last error message (NULL if no error)
    sync_attempts int NOT NULL DEFAULT 0,  -- Consecutive failed attempts (reset on success)
    sync_last_attempt timestamptz,         -- Timestamp of last attempt (for backoff)

    -- Shoot status from Gardener
    shoot_status text,                     -- NULL, 'pending', 'ready', 'error', 'progressing', 'deleting', 'deleted'
    shoot_status_message text,             -- Last status message from Gardener
    shoot_status_updated timestamptz       -- Timestamp of last status check
);

-- Create function to auto-create sync row on cluster INSERT
CREATE OR REPLACE FUNCTION organization.cluster_sync_create_on_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO organization.cluster_sync (cluster_id)
    VALUES (NEW.id);
    RETURN NEW;
END;
$$;

CREATE TRIGGER cluster_sync_create
AFTER INSERT ON organization.clusters
FOR EACH ROW
EXECUTE FUNCTION organization.cluster_sync_create_on_insert();

-- Create function to notify on sync needed (now watches cluster_sync table)
CREATE OR REPLACE FUNCTION organization.cluster_sync_notify()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    -- Notify when synced becomes NULL (new row or state change)
    IF NEW.synced IS NULL THEN
        PERFORM pg_notify('cluster_sync', NEW.cluster_id::text);
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER cluster_sync_notify
AFTER INSERT OR UPDATE OF synced
ON organization.cluster_sync
FOR EACH ROW
EXECUTE FUNCTION organization.cluster_sync_notify();

-- Reset synced when cluster is soft-deleted (triggers re-sync to delete from Gardener)
CREATE OR REPLACE FUNCTION organization.cluster_sync_reset_on_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    -- When clusters.deleted changes from NULL to a timestamp, reset synced
    IF OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
        UPDATE organization.cluster_sync
        SET synced = NULL
        WHERE cluster_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER cluster_sync_reset_on_delete
AFTER UPDATE OF deleted ON organization.clusters
FOR EACH ROW
EXECUTE FUNCTION organization.cluster_sync_reset_on_delete();

-- Partial index for efficient polling of unsynced clusters
CREATE INDEX cluster_sync_idx_unsynced
ON organization.cluster_sync (cluster_id)
WHERE synced IS NULL;

-- Index for status polling queries
CREATE INDEX cluster_sync_idx_status_check
ON organization.cluster_sync (shoot_status_updated)
WHERE synced IS NOT NULL;

COMMENT ON TABLE organization.cluster_sync IS 'Sync state for clusters to Gardener. Separated from clusters table for cleaner separation of concerns.';

-- Grant permissions to cluster-worker role
-- Note: No RLS policy on cluster_sync - worker needs cross-tenant access
-- Note: Worker role is not subject to RLS on clusters table (policy only applies to fun_organization_api)
GRANT USAGE ON SCHEMA organization TO fun_cluster_worker;
GRANT SELECT ON organization.clusters TO fun_cluster_worker;
GRANT SELECT ON organization.tenants TO fun_cluster_worker;
GRANT SELECT, INSERT, UPDATE ON organization.cluster_sync TO fun_cluster_worker;
