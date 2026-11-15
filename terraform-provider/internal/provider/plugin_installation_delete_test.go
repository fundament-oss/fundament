package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

type fakeRunningCluster struct {
	organizationv1connect.UnimplementedClusterServiceHandler
}

func (fakeRunningCluster) GetCluster(context.Context, *organizationv1.GetClusterRequest) (*organizationv1.GetClusterResponse, error) {
	return organizationv1.GetClusterResponse_builder{Cluster: organizationv1.ClusterDetails_builder{
		Id: "cluster-abc", Status: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING,
	}.Build()}.Build(), nil
}

// fakeKubeProxy answers each method with the next of its responses (the last
// one repeats) and records the methods it served.
type fakeKubeProxy struct {
	mu        sync.Mutex
	responses map[string][]fakeResponse
	methods   []string
}

type fakeResponse struct {
	status int
	body   string
}

func (f *fakeKubeProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.methods = append(f.methods, r.Method)
	queue := f.responses[r.Method]
	res := queue[0]
	if len(queue) > 1 {
		f.responses[r.Method] = queue[1:]
	}
	f.mu.Unlock()
	w.WriteHeader(res.status)
	_, _ = w.Write([]byte(res.body))
}

func (f *fakeKubeProxy) served() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.methods...)
}

func pluginResourceWith(t *testing.T, proxy *fakeKubeProxy) *PluginInstallationResource {
	t.Helper()
	kube := httptest.NewServer(proxy)
	t.Cleanup(kube.Close)
	_, handler := organizationv1connect.NewClusterServiceHandler(fakeRunningCluster{})
	org := httptest.NewServer(handler)
	t.Cleanup(org.Close)
	return &PluginInstallationResource{client: &FundamentClient{
		ClusterService:      organizationv1connect.NewClusterServiceClient(org.Client(), org.URL),
		KubeProxyURL:        kube.URL,
		KubeProxyHTTPClient: kube.Client(),
	}}
}

// pinnedInstallation is acme/grafana 1.2.3 on cluster-abc, without config.
func pinnedInstallation(t *testing.T, s schema.Schema) tftypes.Value {
	t.Helper()
	objType := s.Type().TerraformType(context.Background()).(tftypes.Object)
	str := func(v string) tftypes.Value { return tftypes.NewValue(tftypes.String, v) }
	return tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":                str("cluster-abc/acme--grafana"),
		"cluster_id":        str("cluster-abc"),
		"organization_name": str("acme"),
		"plugin_name":       str("grafana"),
		"plugin_version":    str("1.2.3"),
		"definition_hash":   str(unpinnedDefinitionHash),
		"config":            tftypes.NewValue(objType.AttributeTypes["config"], nil),
		"phase":             str("Running"),
	})
}

func deleteInstallation(t *testing.T, r *PluginInstallationResource) *resource.DeleteResponse {
	t.Helper()
	s := pluginInstallationSchema(t)
	state := tfsdk.State{Schema: s, Raw: pinnedInstallation(t, s)}
	resp := &resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)
	return resp
}

const grafanaCR = `{"spec":{"definitionRef":{"organizationName":"acme","pluginName":"grafana","pluginVersion":"1.2.3","definitionHash":"sha256:unknown"}},"status":{"phase":"%s"}}`

func cr(phase string) string { return fmt.Sprintf(grafanaCR, phase) }

func TestDelete_AlreadyGoneSkipsTheWait(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{http.MethodDelete: {{status: http.StatusNotFound}}}}
	resp := deleteInstallation(t, pluginResourceWith(t, proxy))
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.Equal(t, []string{http.MethodDelete}, proxy.served())
}

func TestDelete_WaitsUntilRemoved(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{
		http.MethodDelete: {{status: http.StatusOK, body: cr("Running")}},
		http.MethodGet:    {{status: http.StatusNotFound}},
	}}
	resp := deleteInstallation(t, pluginResourceWith(t, proxy))
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.Equal(t, []string{http.MethodDelete, http.MethodGet}, proxy.served())
}

// A controller that never picks up the deletion leaves the phase as it was.
// Delete then warns rather than holding the destroy for the full timeout.
func TestWaitForPluginGone_IdleControllerEndsTheWait(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{http.MethodGet: {{status: http.StatusOK, body: cr("Failed")}}}}
	r := pluginResourceWith(t, proxy)
	err := r.waitForPluginGone(context.Background(), "cluster-abc", "acme--grafana", time.Millisecond, 20*time.Millisecond)
	require.ErrorIs(t, err, errControllerIdle)
	assert.ErrorContains(t, err, "Failed")
}

// A controller that is uninstalling is given the full timeout.
func TestWaitForPluginGone_TerminatingIsNotIdle(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{http.MethodGet: {{status: http.StatusOK, body: cr("Terminating")}}}}
	r := pluginResourceWith(t, proxy)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	err := r.waitForPluginGone(ctx, "cluster-abc", "acme--grafana", time.Millisecond, 20*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, errControllerIdle)
}

func TestWaitForPluginGone_InterruptedSaysSo(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{http.MethodGet: {{status: http.StatusOK, body: cr("Terminating")}}}}
	r := pluginResourceWith(t, proxy)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	err := r.waitForPluginGone(ctx, "cluster-abc", "acme--grafana", time.Millisecond, time.Minute)
	require.ErrorIs(t, err, context.Canceled)
	assert.ErrorContains(t, err, "interrupted")
	assert.NotContains(t, err.Error(), "timed out")
}

func TestWaitForPluginRunning_InterruptedSaysSo(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{http.MethodGet: {{status: http.StatusOK, body: cr("Deploying")}}}}
	r := pluginResourceWith(t, proxy)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	_, err := r.waitForPluginRunning(ctx, "cluster-abc", "acme--grafana")
	require.ErrorIs(t, err, context.Canceled)
	assert.ErrorContains(t, err, "interrupted")
}

func createInstallation(t *testing.T, r *PluginInstallationResource) *resource.CreateResponse {
	t.Helper()
	s := pluginInstallationSchema(t)
	plan := tfsdk.Plan{Schema: s, Raw: pinnedInstallation(t, s)}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, resp)
	return resp
}

func TestCreate_ConflictWithMatchingSpecAdoptsIt(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{
		http.MethodPost: {{status: http.StatusConflict}},
		http.MethodGet:  {{status: http.StatusOK, body: cr("Running")}},
	}}
	resp := createInstallation(t, pluginResourceWith(t, proxy))
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
}

func TestCreate_ConflictThenGoneFails(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{
		http.MethodPost: {{status: http.StatusConflict}},
		http.MethodGet:  {{status: http.StatusNotFound}},
	}}
	resp := createInstallation(t, pluginResourceWith(t, proxy))
	require.True(t, resp.Diagnostics.HasError())
	assert.Equal(t, "Unable to Read Existing Plugin Installation on 409", resp.Diagnostics.Errors()[0].Summary())
}

func readInstallation(t *testing.T, r *PluginInstallationResource) *resource.ReadResponse {
	t.Helper()
	s := pluginInstallationSchema(t)
	state := tfsdk.State{Schema: s, Raw: pinnedInstallation(t, s)}
	resp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	return resp
}

func TestRead_GoneRemovesFromState(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{http.MethodGet: {{status: http.StatusNotFound}}}}
	resp := readInstallation(t, pluginResourceWith(t, proxy))
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.True(t, resp.State.Raw.IsNull())
}

func TestRead_RefreshesPhase(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{http.MethodGet: {{status: http.StatusOK, body: cr("Degraded")}}}}
	resp := readInstallation(t, pluginResourceWith(t, proxy))
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	var got PluginInstallationResourceModel
	require.False(t, resp.State.Get(context.Background(), &got).HasError())
	assert.Equal(t, "Degraded", got.Phase.ValueString())
	assert.Equal(t, "1.2.3", got.PluginVersion.ValueString())
}

func TestRead_ProxyErrorFails(t *testing.T) {
	proxy := &fakeKubeProxy{responses: map[string][]fakeResponse{http.MethodGet: {{status: http.StatusForbidden, body: "forbidden"}}}}
	resp := readInstallation(t, pluginResourceWith(t, proxy))
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "HTTP 403")
}
