package organization_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Cluster_List_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestAPI(t)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	_, err := client.ListClusters(context.Background(), organizationv1.ListClustersRequest_builder{}.Build())

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func Test_Cluster_List(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	createReq := organizationv1.CreateClusterRequest_builder{
		Name:              "test-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err := client.CreateCluster(createCtx, createReq)
	require.NoError(t, err)

	listReq := organizationv1.ListClustersRequest_builder{}.Build()
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.ListClusters(listCtx, listReq)
	require.NoError(t, err)
	require.Len(t, res.GetClusters(), 1)

	cluster := res.GetClusters()[0]
	assert.Equal(t, "test-cluster", cluster.GetName())
	assert.Equal(t, "eu-west-1", cluster.GetRegion())
	// TODO: kubernetes version missing in cluster?
	assert.Equal(t, organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING, cluster.GetStatus())
}

func Test_Cluster_List_MultiOrg(t *testing.T) {
	t.Parallel()

	orgAID := uuid.New()
	orgBID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgAID, "org-a"),
		WithOrganization(orgBID, "org-b"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgAID, orgBID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	createReq := organizationv1.CreateClusterRequest_builder{
		Name:              "org-a-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgAID.String())

	_, err := client.CreateCluster(createCtx, createReq)
	require.NoError(t, err)

	listReq := organizationv1.ListClustersRequest_builder{}.Build()
	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgBID.String())

	res, err := client.ListClusters(listCtx, listReq)
	require.NoError(t, err)
	assert.Empty(t, res.GetClusters())

	listReqOrgA := organizationv1.ListClustersRequest_builder{}.Build()
	listCtxOrgA, listCallInfoOrgA := connect.NewClientContext(context.Background())
	listCallInfoOrgA.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfoOrgA.RequestHeader().Set("Fun-Organization", orgAID.String())

	resOrgA, err := client.ListClusters(listCtxOrgA, listReqOrgA)
	require.NoError(t, err)
	assert.Len(t, resOrgA.GetClusters(), 1)
}

// The card on /clusters counts what the cluster holds, so the counts have to
// come out of the same rows the detail pages list — and a soft-deleted pool or
// project is gone from both.
func Test_Cluster_List_Counts(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(createCtx, organizationv1.CreateClusterRequest_builder{
		Name:              "counted-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	require.NoError(t, err)

	clusterID := createRes.GetClusterId()

	for _, name := range []string{"workers", "spot"} {
		_, err = env.adminPool.Exec(t.Context(),
			"INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max) VALUES ($1, $2, 'm1.small', 1, 3)",
			clusterID, name,
		)
		require.NoError(t, err)
	}

	_, err = env.adminPool.Exec(t.Context(),
		"INSERT INTO tenant.node_pools (cluster_id, name, machine_type, autoscale_min, autoscale_max, deleted) VALUES ($1, 'retired', 'm1.small', 1, 3, NOW())",
		clusterID,
	)
	require.NoError(t, err)

	projectClient := organizationv1connect.NewProjectServiceClient(env.server.Client(), env.server.URL)
	projectCtx, projectCallInfo := connect.NewClientContext(context.Background())
	projectCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	projectCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = projectClient.CreateProject(projectCtx, organizationv1.CreateProjectRequest_builder{
		ClusterId: clusterID,
		Name:      "burgerzaken",
	}.Build())
	require.NoError(t, err)

	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.ListClusters(listCtx, organizationv1.ListClustersRequest_builder{}.Build())
	require.NoError(t, err)
	require.Len(t, res.GetClusters(), 1)

	cluster := res.GetClusters()[0]
	assert.Equal(t, int32(2), cluster.GetNodePoolCount())
	assert.Equal(t, int32(1), cluster.GetProjectCount())
}

// The list comes back newest first and the order has to be stable across
// requests. Rows created in one transaction share a timestamp, so the id (a
// UUIDv7, time-ordered) breaks the tie. Presentation order, such as by name,
// is the console's business.
func Test_Cluster_List_OrderedByCreation(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	// Deliberately not alphabetical, so an order by name would fail this test.
	created := []string{"charlie", "alpha", "delta", "bravo"}
	for _, name := range created {
		createCtx, createCallInfo := connect.NewClientContext(context.Background())
		createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
		createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

		_, err := client.CreateCluster(createCtx, organizationv1.CreateClusterRequest_builder{
			Name:              name,
			Region:            "eu-west-1",
			KubernetesVersion: "1.28",
		}.Build())
		require.NoError(t, err)
	}

	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.ListClusters(listCtx, organizationv1.ListClustersRequest_builder{}.Build())
	require.NoError(t, err)

	names := make([]string, 0, len(res.GetClusters()))
	for _, cluster := range res.GetClusters() {
		names = append(names, cluster.GetName())
	}

	assert.Equal(t, []string{"bravo", "delta", "alpha", "charlie"}, names)
}

// Two clusters with the same timestamp: only the id can put them in order. The
// API gives every row its own timestamp, so the rows go in directly, and in the
// order that a query without the tie-breaker would most likely hand back.
func Test_Cluster_List_TieBreaksOnID(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	env := newTestAPI(t,
		WithOrganization(orgID, "test-org"),
		WithUser(&UserArgs{
			ID:     userID,
			Name:   "test-user",
			OrgIDs: []uuid.UUID{orgID},
		}),
	)

	first, err := uuid.NewV7()
	require.NoError(t, err)
	second, err := uuid.NewV7()
	require.NoError(t, err)
	// UUIDv7 is time-ordered, but the order between two made this close
	// together is decided by the counter bits, so it is read off, not assumed.
	lower, higher := first, second
	if bytes.Compare(first[:], second[:]) > 0 {
		lower, higher = second, first
	}

	created := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	for _, cluster := range []struct {
		id   uuid.UUID
		name string
	}{{lower, "lower-id"}, {higher, "higher-id"}} {
		_, err = env.adminPool.Exec(t.Context(),
			"INSERT INTO tenant.clusters (id, organization_id, name, region, kubernetes_version, created) VALUES ($1, $2, $3, 'eu-west-1', '1.28', $4)",
			cluster.id, orgID, cluster.name, created,
		)
		require.NoError(t, err)
	}

	token := env.createAuthnToken(t, userID)
	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	listCtx, listCallInfo := connect.NewClientContext(context.Background())
	listCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	listCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.ListClusters(listCtx, organizationv1.ListClustersRequest_builder{}.Build())
	require.NoError(t, err)

	names := make([]string, 0, len(res.GetClusters()))
	for _, cluster := range res.GetClusters() {
		names = append(names, cluster.GetName())
	}

	assert.Equal(t, []string{"higher-id", "lower-id"}, names)
}
