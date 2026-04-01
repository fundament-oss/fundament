package organization_test

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	createReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name:              "not-ready-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(context.Background(), createReq)
	require.NoError(t, err)

	clusterID := createRes.Msg.GetClusterId()

	// GetKubeconfig should fail because shoot fields are not populated.
	kcReq := connect.NewRequest(organizationv1.GetKubeconfigRequest_builder{
		ClusterId: clusterID,
	}.Build())
	kcReq.Header().Set("Authorization", "Bearer "+token)
	kcReq.Header().Set("Fun-Organization", orgID.String())

	_, err = client.GetKubeconfig(context.Background(), kcReq)

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
	)

	token := env.createAuthnToken(t, userID)

	client := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)

	// Create a cluster.
	createReq := connect.NewRequest(organizationv1.CreateClusterRequest_builder{
		Name:              "ready-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build())
	createReq.Header().Set("Authorization", "Bearer "+token)
	createReq.Header().Set("Fun-Organization", orgID.String())

	createRes, err := client.CreateCluster(context.Background(), createReq)
	require.NoError(t, err)

	clusterID := createRes.Msg.GetClusterId()

	// Simulate shoot becoming ready by populating shoot fields directly in the DB.
	_, err = env.adminPool.Exec(t.Context(),
		"UPDATE tenant.clusters SET shoot_api_server_url = $1, shoot_ca_data = $2 WHERE id = $3",
		"https://api.test.example.com", "dGVzdC1jYS1kYXRh", clusterID,
	)
	require.NoError(t, err)

	// Now GetKubeconfig should succeed.
	kcReq := connect.NewRequest(organizationv1.GetKubeconfigRequest_builder{
		ClusterId: clusterID,
	}.Build())
	kcReq.Header().Set("Authorization", "Bearer "+token)
	kcReq.Header().Set("Fun-Organization", orgID.String())

	res, err := client.GetKubeconfig(context.Background(), kcReq)
	require.NoError(t, err)

	kc := res.Msg.GetKubeconfigContent()

	// Without KubeAPIProxyURL configured, falls back to direct shoot URL.
	assert.Contains(t, kc, "server: https://api.test.example.com")
	assert.NotContains(t, kc, "certificate-authority-data:")
	assert.Contains(t, kc, "fundament-"+clusterID)
	assert.Contains(t, kc, "fundament-user-"+clusterID)
	assert.Contains(t, kc, "command: functl")
	assert.Contains(t, kc, "- cluster")
	assert.Contains(t, kc, "- token")
	assert.Contains(t, kc, "- "+clusterID)
	assert.Contains(t, kc, "client.authentication.k8s.io/v1")

	// Verify it's valid YAML (basic structure check).
	assert.True(t, strings.HasPrefix(kc, "apiVersion: v1"))
}
