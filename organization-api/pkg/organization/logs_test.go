package organization_test

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

type logsEnv struct {
	env       *testEnv
	token     string
	orgID     uuid.UUID
	clusterID uuid.UUID
	client    organizationv1connect.LogsServiceClient
}

func newLogsEnv(t *testing.T) *logsEnv {
	t.Helper()

	orgID := uuid.New()
	userID := uuid.New()
	env := newTestAPI(t,
		WithOrganization(orgID, "logs-org"),
		WithUser(&UserArgs{ID: userID, Name: "logs-user", OrgIDs: []uuid.UUID{orgID}}),
		WithMockLogs(logs.NewMockClient()),
	)
	token := env.createAuthnToken(t, userID)

	clusterClient := organizationv1connect.NewClusterServiceClient(env.server.Client(), env.server.URL)
	req := organizationv1.CreateClusterRequest_builder{
		Name:              "logs-cluster",
		Region:            "eu-west-1",
		KubernetesVersion: "1.28",
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+token)
	callInfo.RequestHeader().Set("Fun-Organization", orgID.String())
	res, err := clusterClient.CreateCluster(ctx, req)
	require.NoError(t, err)

	return &logsEnv{
		env:       env,
		token:     token,
		orgID:     orgID,
		clusterID: uuid.MustParse(res.GetClusterId()),
		client:    organizationv1connect.NewLogsServiceClient(env.server.Client(), env.server.URL),
	}
}

func (l *logsEnv) authed(h http.Header) {
	h.Set("Authorization", "Bearer "+l.token)
	h.Set("Fun-Organization", l.orgID.String())
}

// Spec scenario "Mock mode renders": QueryLogs against the mock backend
// returns populated entries marked as the full-featured backend.
func Test_Logs_Query_MockBackend(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: l.clusterID.String(),
		Limit:     25,
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.QueryLogs(ctx, req)
	require.NoError(t, err)

	entries := res.GetEntries()
	require.Len(t, entries, 25)
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_LOKI, res.GetBackend())
	// Newest first.
	first := entries[0].GetTimestamp().AsTime()
	second := entries[1].GetTimestamp().AsTime()
	assert.True(t, first.After(second))
	for _, e := range entries {
		assert.NotEmpty(t, e.GetMessage())
		assert.NotEmpty(t, e.GetNamespace())
	}
}

func Test_Logs_Query_NamespaceFilter(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: l.clusterID.String(),
		Namespace: "kube-system",
		Limit:     10,
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.QueryLogs(ctx, req)
	require.NoError(t, err)
	require.NotEmpty(t, res.GetEntries())
	for _, e := range res.GetEntries() {
		assert.Equal(t, "kube-system", e.GetNamespace())
	}
}

// Spec scenario "Unknown cluster": querying a cluster id that does not exist
// fails with not-found instead of silently returning empty results.
func Test_Logs_Query_UnknownCluster_NotFound(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.QueryLogsRequest_builder{
		ClusterId: uuid.NewString(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	_, err := l.client.QueryLogs(ctx, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func Test_Logs_GetLogLabels_MockBackend(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.GetLogLabelsRequest_builder{
		ClusterId: l.clusterID.String(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	res, err := l.client.GetLogLabels(ctx, req)
	require.NoError(t, err)
	assert.Contains(t, res.GetNamespaces(), "kube-system")
	assert.NotEmpty(t, res.GetPods())
	assert.Equal(t, organizationv1.LogBackend_LOG_BACKEND_LOKI, res.GetBackend())
}

// Regression: GetLogLabels verifies the cluster exists (the original branch
// skipped this check).
func Test_Logs_GetLogLabels_UnknownCluster_NotFound(t *testing.T) {
	t.Parallel()
	l := newLogsEnv(t)

	req := organizationv1.GetLogLabelsRequest_builder{
		ClusterId: uuid.NewString(),
	}.Build()
	ctx, callInfo := connect.NewClientContext(context.Background())
	l.authed(callInfo.RequestHeader())

	_, err := l.client.GetLogLabels(ctx, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
