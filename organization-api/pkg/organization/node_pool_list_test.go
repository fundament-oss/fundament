package organization_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// fakeNodePrometheus answers the three node queries ListNodePools issues, with
// two nodes in "workers" (one of them not ready) and one in "spot".
func fakeNodePrometheus(t *testing.T) *httptest.Server {
	t.Helper()

	series := func(entries ...string) string {
		return fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[%s]}}`, strings.Join(entries, ","))
	}
	sample := func(metric, value string) string {
		return fmt.Sprintf(`{"metric":%s,"value":[1700000000,%q]}`, metric, value)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		switch {
		case strings.Contains(query, "kube_node_labels"):
			_, _ = fmt.Fprint(w, series(
				sample(`{"node":"n1","label_worker_gardener_cloud_pool":"workers"}`, "1"),
				sample(`{"node":"n2","label_worker_gardener_cloud_pool":"workers"}`, "1"),
				sample(`{"node":"n3","label_worker_gardener_cloud_pool":"spot"}`, "1"),
			))
		case strings.Contains(query, "kube_node_status_condition"):
			_, _ = fmt.Fprint(w, series(
				sample(`{"node":"n1","condition":"Ready"}`, "1"),
				sample(`{"node":"n2","condition":"Ready"}`, "0"),
				sample(`{"node":"n3","condition":"Ready"}`, "1"),
			))
		case strings.Contains(query, "kube_node_info"):
			_, _ = fmt.Fprint(w, series(
				sample(`{"node":"n1","kubelet_version":"v1.31.4"}`, "1"),
				sample(`{"node":"n2","kubelet_version":"v1.31.4"}`, "1"),
				sample(`{"node":"n3","kubelet_version":"v1.31.4"}`, "1"),
			))
		default:
			_, _ = fmt.Fprint(w, series())
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// nodePoolTestCluster creates a cluster with a "workers" and a "spot" pool and
// returns the cluster id and an authorized client.
func nodePoolTestCluster(t *testing.T, env *testEnv, orgID uuid.UUID, token string) (string, organizationv1connect.ClusterServiceClient, context.Context) {
	t.Helper()

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{
		Name:              "node-pool-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	clusterID := res.GetClusterId()
	for _, name := range []string{"workers", "spot"} {
		_, err = env.adminPool.Exec(t.Context(),
			"INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max) VALUES ($1, $2, 'm1.small', 1, 3)",
			clusterID, name,
		)
		require.NoError(t, err)
	}

	return clusterID, client, ctx
}

// The pools in the database are the desired shape; their status, node count and
// version come from what the cluster reports.
func Test_NodePool_List_ReportsClusterState(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
		WithPrometheusBackend(fakeNodePrometheus(t).URL, nil),
	)
	token := env.createAuthnToken(t, userID)

	clusterID, client, ctx := nodePoolTestCluster(t, env, orgID, token)

	res, err := client.ListNodePools(ctx, organizationv1.ListNodePoolsRequest_builder{
		ClusterId: clusterID,
	}.Build())
	require.NoError(t, err)
	require.Len(t, res.GetNodePools(), 2)

	pools := make(map[string]*organizationv1.NodePool, len(res.GetNodePools()))
	for _, pool := range res.GetNodePools() {
		pools[pool.GetName()] = pool
	}

	workers := pools["workers"]
	require.NotNil(t, workers)
	assert.Equal(t, int32(2), workers.GetCurrentNodes())
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_DEGRADED, workers.GetStatus())
	assert.Equal(t, "v1.31.4", workers.GetVersion())

	spot := pools["spot"]
	require.NotNil(t, spot)
	assert.Equal(t, int32(1), spot.GetCurrentNodes())
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_HEALTHY, spot.GetStatus())
}

// A cluster with no reachable metrics backend still lists its pools; only the
// live fields fall back to unknown.
func Test_NodePool_List_DegradesWithoutMetrics(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{ID: userID, Name: "test-user", OrgIDs: []uuid.UUID{orgID}}),
	)
	token := env.createAuthnToken(t, userID)

	clusterID, client, ctx := nodePoolTestCluster(t, env, orgID, token)

	res, err := client.ListNodePools(ctx, organizationv1.ListNodePoolsRequest_builder{
		ClusterId: clusterID,
	}.Build())
	require.NoError(t, err)
	require.Len(t, res.GetNodePools(), 2)

	for _, pool := range res.GetNodePools() {
		assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, pool.GetStatus())
		assert.Equal(t, int32(0), pool.GetCurrentNodes())
		assert.Empty(t, pool.GetVersion())
	}
}
