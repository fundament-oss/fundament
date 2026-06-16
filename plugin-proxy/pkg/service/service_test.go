package service

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	pluginproxyv1 "github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, pluginsv1.AddToScheme(s), "add plugins scheme")
	return s
}

// fakeClusterAccess returns a fixed client for every cluster_id.
type fakeClusterAccess struct {
	client client.Client
	orgID  uuid.UUID
}

func (f *fakeClusterAccess) ForCluster(_ context.Context, _ uuid.UUID) (*ClusterTarget, error) {
	return &ClusterTarget{Client: f.client, OrganizationID: f.orgID}, nil
}

func testService(c client.Client) *Service {
	return &Service{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Cluster: &fakeClusterAccess{client: c, orgID: MockOrganizationID},
	}
}

func testPluginCR() *pluginsv1.PluginInstallation {
	return &pluginsv1.PluginInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name: MockPluginName,
			UID:  types.UID(MockInstallationID.String()),
		},
		Spec: pluginsv1.PluginInstallationSpec{
			Image: "ghcr.io/example/" + MockPluginName + ":" + MockPluginVersion,
			DefinitionRef: pluginsv1.DefinitionRef{
				PluginName:     MockPluginName,
				PluginVersion:  MockPluginVersion,
				DefinitionHash: MockPluginHash,
			},
		},
		Status: pluginsv1.PluginInstallationStatus{Phase: pluginsv1.PluginPhaseRunning},
	}
}

func TestGetInstallationManifest_ReturnsIdentity(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(testPluginCR()).Build()
	svc := testService(c)

	resp, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      MockClusterID.String(),
			InstallationId: MockInstallationID.String(),
		}.Build()))
	require.NoError(t, err, "GetInstallationManifest")

	msg := resp.Msg
	assert.Equal(t, MockPluginName, msg.GetPluginName())
	assert.Equal(t, MockPluginVersion, msg.GetPluginVersion())
	assert.Equal(t, MockPluginHash, msg.GetDefinitionHash())
	assert.Equal(t, MockOrganizationID.String(), msg.GetOrganizationId())
	assert.Equal(t, "Running", msg.GetStatus())
}

// Mints keep working through teardown so plugin tokens can read state during
// deletion. A CR with a deletionTimestamp or in the Terminating phase still
// resolves; the phase is passed through for audit.
func TestGetInstallationManifest_DeletingReturnsManifest(t *testing.T) {
	now := metav1.Now()
	cr := testPluginCR()
	cr.DeletionTimestamp = &now
	cr.Finalizers = []string{"plugins.fundament.io/finalizer"}
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(cr).Build()
	svc := testService(c)

	resp, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      MockClusterID.String(),
			InstallationId: MockInstallationID.String(),
		}.Build()))
	require.NoError(t, err, "GetInstallationManifest")
	assert.Equal(t, MockPluginName, resp.Msg.GetPluginName())
}

func TestGetInstallationManifest_TerminatingPhaseReturnsManifest(t *testing.T) {
	cr := testPluginCR()
	cr.Status.Phase = pluginsv1.PluginPhaseTerminating
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(cr).Build()
	svc := testService(c)

	resp, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      MockClusterID.String(),
			InstallationId: MockInstallationID.String(),
		}.Build()))
	require.NoError(t, err, "GetInstallationManifest")
	assert.Equal(t, string(pluginsv1.PluginPhaseTerminating), resp.Msg.GetStatus())
}

// Protovalidate rejects non-UUID inputs at the interceptor; without this
// gate, the handler would forward bad input to the kube client.
func TestGetInstallationManifest_MalformedUUID_InvalidArgument(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(testPluginCR()).Build()
	svc := testService(c)

	mux := http.NewServeMux()
	path, handler := pluginproxyv1connect.NewPluginInstallationServiceHandler(svc,
		connect.WithInterceptors(validate.NewInterceptor()))
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	rpcClient := pluginproxyv1connect.NewPluginInstallationServiceClient(srv.Client(), srv.URL)
	_, err := rpcClient.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "not-a-uuid",
			InstallationId: MockInstallationID.String(),
		}.Build()))
	require.Error(t, err, "expected error for malformed cluster_id")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestGetInstallationManifest_NotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	svc := testService(c)

	_, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      MockClusterID.String(),
			InstallationId: "11111111-0000-0000-0000-000000000000",
		}.Build()))
	require.Error(t, err, "expected error for missing CR")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestMockClusterAccess_UnknownClusterUnavailable(t *testing.T) {
	svc := service(t)
	_, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "00000000-0000-0000-0000-0000000000ff",
			InstallationId: MockInstallationID.String(),
		}.Build()))
	require.Error(t, err, "expected error for unknown cluster")
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

func service(t *testing.T) *Service {
	t.Helper()
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), NewMockClusterAccess())
}
