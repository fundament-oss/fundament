package testpg

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Seed inserts rows as postgres, around RLS. Every helper returns the new id.
type Seed struct {
	t   *testing.T
	env *Env
}

func (e *Env) Seed(t *testing.T) *Seed {
	t.Helper()
	return &Seed{t: t, env: e}
}

func (s *Seed) id(query string, args ...any) uuid.UUID {
	s.t.Helper()
	var id uuid.UUID
	require.NoError(s.t, s.env.Super.QueryRow(s.t.Context(), query, args...).Scan(&id))
	return id
}

func (s *Seed) exec(query string, args ...any) {
	s.t.Helper()
	_, err := s.env.Super.Exec(s.t.Context(), query, args...)
	require.NoError(s.t, err)
}

func (s *Seed) Organization(name, alias string) uuid.UUID {
	s.t.Helper()
	return s.id(`INSERT INTO tenant.organizations (name, alias) VALUES ($1, $2) RETURNING id`, name, alias)
}

func (s *Seed) Cluster(orgID uuid.UUID, name string) uuid.UUID {
	s.t.Helper()
	return s.id(`INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version)
		VALUES ($1, $2, 'eu-west-1', '1.31.1') RETURNING id`, orgID, name)
}

// Project inserts a project with the admin member a deferred trigger requires.
func (s *Seed) Project(clusterID uuid.UUID, name string) uuid.UUID {
	s.t.Helper()
	return s.id(`WITH p AS (
		     INSERT INTO tenant.projects (cluster_id, name, alias) VALUES ($1, $2, $2) RETURNING id
		 ), u AS (
		     INSERT INTO tenant.users (name) VALUES ($2 || '-admin-' || gen_random_uuid()) RETURNING id
		 ), m AS (
		     INSERT INTO tenant.project_members (project_id, user_id, role)
		     SELECT p.id, u.id, 'admin' FROM p, u
		 )
		 SELECT id FROM p`, clusterID, name)
}

func (s *Seed) Namespace(projectID uuid.UUID, name string) uuid.UUID {
	s.t.Helper()
	return s.id(`INSERT INTO tenant.namespaces (project_id, name) VALUES ($1, $2) RETURNING id`, projectID, name)
}

func (s *Seed) NodePool(clusterID uuid.UUID, name string, scaleMin, scaleMax int32) uuid.UUID {
	s.t.Helper()
	return s.id(`INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max)
		VALUES ($1, $2, 'n1-standard-2', $3, $4) RETURNING id`, clusterID, name, scaleMin, scaleMax)
}

// SoftDelete marks a tenant row deleted.
func (s *Seed) SoftDelete(table string, id uuid.UUID) {
	s.t.Helper()
	s.exec(`UPDATE tenant.`+table+` SET deleted = now() WHERE id = $1`, id)
}

// Ready marks a cluster ready and synced, its status changed age ago.
func (s *Seed) Ready(clusterID uuid.UUID, age time.Duration) {
	s.t.Helper()
	s.exec(`UPDATE tenant.clusters
		SET shoot_status = 'ready', shoot_health = 'healthy', outbox_status = 'completed',
		    shoot_status_updated = now() - make_interval(secs => $2)
		WHERE id = $1`, clusterID, age.Seconds())
}

// Status sets shoot status columns directly.
func (s *Seed) Status(clusterID uuid.UUID, status, health string, updating bool) {
	s.t.Helper()
	s.exec(`UPDATE tenant.clusters
		SET shoot_status = $2, shoot_health = NULLIF($3, ''), shoot_updating = $4
		WHERE id = $1`, clusterID, status, health, updating)
}

// Limits gives an organization node caps; nil leaves one unlimited.
func (s *Seed) Limits(orgID uuid.UUID, maxNodesPerCluster, maxNodePoolsPerCluster, maxNodesPerNodePool *int32) {
	s.t.Helper()
	s.exec(`INSERT INTO tenant.organization_limits
		(organization_id, max_nodes_per_cluster, max_node_pools_per_cluster, max_nodes_per_node_pool)
		VALUES ($1, $2, $3, $4)`, orgID, maxNodesPerCluster, maxNodePoolsPerCluster, maxNodesPerNodePool)
}

// ClusterSnapshot records an observation observedAgo ago.
func (s *Seed) ClusterSnapshot(clusterID uuid.UUID, observedAgo time.Duration, nodes, nodesReady, pods, podAllocatable int32) {
	s.t.Helper()
	s.exec(`INSERT INTO tenant.cluster_snapshots
		(cluster_id, observed, nodes, nodes_ready, pods, pod_allocatable, cpu_allocatable_m, memory_allocatable_mi)
		VALUES ($1, now() - make_interval(secs => $2), $3, $4, $5, $6, $3 * 4000, $3 * 16384)`,
		clusterID, observedAgo.Seconds(), nodes, nodesReady, pods, podAllocatable)
}

func (s *Seed) NodePoolSnapshot(poolID uuid.UUID, nodes, nodesReady int32) {
	s.t.Helper()
	s.exec(`INSERT INTO tenant.node_pool_snapshots (node_pool_id, observed, nodes, nodes_ready)
		VALUES ($1, now(), $2, $3)`, poolID, nodes, nodesReady)
}

func (s *Seed) NamespaceSnapshot(namespaceID uuid.UUID, pods int32) {
	s.t.Helper()
	s.exec(`INSERT INTO tenant.namespace_snapshots (namespace_id, observed, pods)
		VALUES ($1, now(), $2)`, namespaceID, pods)
}

// GardenerShoot records a live shoot last seen seenAgo ago.
func (s *Seed) GardenerShoot(clusterID uuid.UUID, name, version string, seenAgo time.Duration) uuid.UUID {
	s.t.Helper()
	return s.id(`INSERT INTO tenant.gardener_shoots
		(shoot_uid, shoot_name, shoot_namespace, cluster_id, kubernetes_version, region, shoot_created, first_seen, last_seen)
		VALUES (gen_random_uuid()::text, $2, 'garden-test', $1, $3, 'eu-west-1', now() - interval '1 day',
		        now() - make_interval(secs => $4), now() - make_interval(secs => $4))
		RETURNING id`, clusterID, name, version, seenAgo.Seconds())
}
