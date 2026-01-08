-- Drop triggers and functions
DROP TRIGGER IF EXISTS cluster_sync_reset_on_delete ON organization.clusters;
DROP TRIGGER IF EXISTS cluster_sync_notify ON organization.cluster_sync;
DROP TRIGGER IF EXISTS cluster_sync_create ON organization.clusters;
DROP FUNCTION IF EXISTS organization.cluster_sync_reset_on_delete();
DROP FUNCTION IF EXISTS organization.cluster_sync_notify();
DROP FUNCTION IF EXISTS organization.cluster_sync_create_on_insert();

-- Drop indexes
DROP INDEX IF EXISTS organization.cluster_sync_idx_status_check;
DROP INDEX IF EXISTS organization.cluster_sync_idx_unsynced;

-- Drop the cluster_sync table
DROP TABLE IF EXISTS organization.cluster_sync;

-- Drop the cluster-worker role
DROP ROLE IF EXISTS fun_cluster_worker;
