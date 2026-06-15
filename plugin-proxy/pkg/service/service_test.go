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
	if err := pluginsv1.AddToScheme(s); err != nil {
		t.Fatalf("add plugins scheme: %v", err)
	}
	return s
}

// fakeClusterAccess returns a fixed client for every cluster_id.
type fakeClusterAccess struct {
	client client.Client
	orgID  string
}

func (f *fakeClusterAccess) ForCluster(_ context.Context, _ string) (*ClusterTarget, error) {
	return &ClusterTarget{Client: f.client, OrganizationID: f.orgID}, nil
}

func testService(c client.Client) *Service {
	return &Service{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Cluster: &fakeClusterAccess{client: c, orgID: "org-uuid"},
	}
}

func certManagerCR() *pluginsv1.PluginInstallation {
	return &pluginsv1.PluginInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
			UID:  types.UID(MockInstallationUID),
		},
		Spec: pluginsv1.PluginInstallationSpec{
			Image: "ghcr.io/example/cert-manager:v1.17.2",
			DefinitionRef: pluginsv1.DefinitionRef{
				PluginName:     "cert-manager",
				PluginVersion:  "v1.17.2",
				DefinitionHash: "sha256:1f3c9a",
			},
		},
		Status: pluginsv1.PluginInstallationStatus{Phase: pluginsv1.PluginPhaseRunning},
	}
}

func TestGetInstallationManifest_ReturnsIdentity(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(certManagerCR()).Build()
	svc := testService(c)

	resp, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "cluster-uuid",
			InstallationId: MockInstallationUID,
		}.Build()))
	if err != nil {
		t.Fatalf("GetInstallationManifest: %v", err)
	}

	msg := resp.Msg
	if msg.GetPluginName() != "cert-manager" {
		t.Errorf("plugin_name = %q", msg.GetPluginName())
	}
	if msg.GetPluginVersion() != "v1.17.2" {
		t.Errorf("plugin_version = %q", msg.GetPluginVersion())
	}
	if msg.GetDefinitionHash() != "sha256:1f3c9a" {
		t.Errorf("definition_hash = %q", msg.GetDefinitionHash())
	}
	if msg.GetOrganizationId() != "org-uuid" {
		t.Errorf("organization_id = %q", msg.GetOrganizationId())
	}
	if msg.GetStatus() != "Running" {
		t.Errorf("status = %q", msg.GetStatus())
	}
}

// Mints keep working through teardown so plugin tokens can read state during
// deletion. A CR with a deletionTimestamp or in the Terminating phase still
// resolves; the phase is passed through for audit.
func TestGetInstallationManifest_DeletingReturnsManifest(t *testing.T) {
	now := metav1.Now()
	cr := certManagerCR()
	cr.DeletionTimestamp = &now
	cr.Finalizers = []string{"plugins.fundament.io/finalizer"}
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(cr).Build()
	svc := testService(c)

	resp, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "cluster-uuid",
			InstallationId: MockInstallationUID,
		}.Build()))
	if err != nil {
		t.Fatalf("GetInstallationManifest: %v", err)
	}
	if resp.Msg.GetPluginName() != "cert-manager" {
		t.Errorf("plugin_name = %q", resp.Msg.GetPluginName())
	}
}

func TestGetInstallationManifest_TerminatingPhaseReturnsManifest(t *testing.T) {
	cr := certManagerCR()
	cr.Status.Phase = pluginsv1.PluginPhaseTerminating
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(cr).Build()
	svc := testService(c)

	resp, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "cluster-uuid",
			InstallationId: MockInstallationUID,
		}.Build()))
	if err != nil {
		t.Fatalf("GetInstallationManifest: %v", err)
	}
	if got := resp.Msg.GetStatus(); got != string(pluginsv1.PluginPhaseTerminating) {
		t.Errorf("status = %q, want Terminating", got)
	}
}

// Protovalidate rejects non-UUID inputs at the interceptor; without this
// gate, the handler would forward bad input to the kube client.
func TestGetInstallationManifest_MalformedUUID_InvalidArgument(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(certManagerCR()).Build()
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
			InstallationId: MockInstallationUID,
		}.Build()))
	if err == nil {
		t.Fatal("expected error for malformed cluster_id")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestGetInstallationManifest_NotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	svc := testService(c)

	_, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "cluster-uuid",
			InstallationId: "11111111-0000-0000-0000-000000000000",
		}.Build()))
	if err == nil {
		t.Fatal("expected error for missing CR")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
	}
}

func TestMockClusterAccess_UnknownClusterUnavailable(t *testing.T) {
	svc := service(t)
	_, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "00000000-0000-0000-0000-0000000000ff",
			InstallationId: MockInstallationUID,
		}.Build()))
	if err == nil {
		t.Fatal("expected error for unknown cluster")
	}
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("code = %v, want Unavailable", connect.CodeOf(err))
	}
}

func service(t *testing.T) *Service {
	t.Helper()
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), NewMockClusterAccess())
}
