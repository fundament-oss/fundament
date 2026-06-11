package installation

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	pluginproxyv1 "github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1"
)

const testInstallationUID = "00000000-0000-0000-0000-000000000001"

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := pluginsv1.AddToScheme(s); err != nil {
		t.Fatalf("add plugins scheme: %v", err)
	}
	return s
}

func testService(c client.Client) *Service {
	return &Service{
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClusterClient:   func(_ context.Context, _ string) (client.Client, error) { return c, nil },
		OrgIDForCluster: func(_ context.Context, _ string) (string, error) { return "org-uuid", nil },
	}
}

func certManagerCR() *pluginsv1.PluginInstallation {
	return &pluginsv1.PluginInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cert-manager",
			UID:  types.UID(testInstallationUID),
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
			InstallationId: testInstallationUID,
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

func TestGetInstallationManifest_TerminatingReturnsFailedPrecondition(t *testing.T) {
	now := metav1.Now()
	cr := certManagerCR()
	cr.DeletionTimestamp = &now
	cr.Finalizers = []string{"plugins.fundament.io/finalizer"}
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(cr).Build()
	svc := testService(c)

	_, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "cluster-uuid",
			InstallationId: testInstallationUID,
		}.Build()))
	if err == nil {
		t.Fatal("expected error for terminating CR")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
}

func TestGetInstallationManifest_TerminatingPhaseReturnsFailedPrecondition(t *testing.T) {
	cr := certManagerCR()
	cr.Status.Phase = pluginsv1.PluginPhaseTerminating
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(cr).Build()
	svc := testService(c)

	_, err := svc.GetInstallationManifest(context.Background(),
		connect.NewRequest(pluginproxyv1.GetInstallationManifestRequest_builder{
			ClusterId:      "cluster-uuid",
			InstallationId: testInstallationUID,
		}.Build()))
	if err == nil {
		t.Fatal("expected error for terminating-phase CR")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
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
