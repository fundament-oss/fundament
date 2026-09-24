package organization_test

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

func Test_GetKubeconfig_ClusterNotReady(t *testing.T) {
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

	// Create a cluster (shoot fields are not populated yet).
	createReq := organizationv1.CreateClusterRequest_builder{
		Name:              "not-ready-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(createCtx, createReq)
	require.NoError(t, err)

	clusterID := createRes.GetClusterId()

	// GetKubeconfig should fail because shoot fields are not populated.
	kcReq := organizationv1.GetKubeconfigRequest_builder{
		ClusterId: clusterID,
	}.Build()
	kcCtx, kcCallInfo := connect.NewClientContext(context.Background())
	kcCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	kcCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	_, err = client.GetKubeconfig(kcCtx, kcReq)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Contains(t, connectErr.Message(), "cluster not ready")
}

func Test_GetKubeconfig_Ready(t *testing.T) {
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
		WithKubeAPIProxy("https://k8s-api.example.com"),
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	// Create a cluster.
	createReq := organizationv1.CreateClusterRequest_builder{
		Name:              "ready-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	createCtx, createCallInfo := connect.NewClientContext(context.Background())
	createCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	createCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(createCtx, createReq)
	require.NoError(t, err)

	clusterID := createRes.GetClusterId()

	// Simulate shoot becoming ready.
	_, err = env.adminPool.Exec(t.Context(),
		"UPDATE tenant.clusters SET shoot_status = 'ready' WHERE id = $1",
		clusterID,
	)
	require.NoError(t, err)

	// Now GetKubeconfig should succeed.
	kcReq := organizationv1.GetKubeconfigRequest_builder{
		ClusterId: clusterID,
	}.Build()
	kcCtx, kcCallInfo := connect.NewClientContext(context.Background())
	kcCallInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	kcCallInfo.RequestHeader().Set("Fun-Organization", orgID.String())

	res, err := client.GetKubeconfig(kcCtx, kcReq)
	require.NoError(t, err)

	kc := res.GetKubeconfigContent()

	// Kubeconfig should point at the proxy.
	assert.Contains(t, kc, "server: https://k8s-api.example.com/clusters/"+clusterID)
	assert.NotContains(t, kc, "insecure-skip-tls-verify")
	cfg, err := clientcmd.Load([]byte(kc))
	require.NoError(t, err, "a loadable kubeconfig")
	assert.Equal(t, "test-org--ready-cluster", cfg.CurrentContext, "entries named <org>--<cluster>")
	require.Contains(t, cfg.Contexts, "test-org--ready-cluster")
	assert.Equal(t, "test-org--ready-cluster", cfg.Contexts["test-org--ready-cluster"].Cluster)
	assert.Equal(t, "test-org--ready-cluster", cfg.Contexts["test-org--ready-cluster"].AuthInfo)
	assert.NotContains(t, kc, "fundament-"+clusterID, "no uuid-based entry names")
	assert.Contains(t, kc, "command: functl")
	assert.Contains(t, kc, "- cluster")
	assert.Contains(t, kc, "- token")
	assert.Contains(t, kc, "- "+clusterID)
	assert.Contains(t, kc, "client.authentication.k8s.io/v1")

	// Verify it's valid YAML (basic structure check).
	assert.True(t, strings.HasPrefix(kc, "apiVersion: v1"))
}
