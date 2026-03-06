package sdktesting

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	pluginsdk "github.com/fundament-oss/fundament/plugin-sdk"
	pb "github.com/fundament-oss/fundament/plugin-sdk/metadata/proto/gen/v1"
)

// testPlugin is a minimal plugin for integration testing.
type testPlugin struct {
	started  chan struct{}
	shutdown chan struct{}
}

func newTestPlugin() *testPlugin {
	return &testPlugin{
		started:  make(chan struct{}),
		shutdown: make(chan struct{}),
	}
}

func (p *testPlugin) Definition() pluginsdk.PluginDefinition {
	return pluginsdk.PluginDefinition{
		Metadata: pluginsdk.PluginMetadata{
			Name:        "test-plugin",
			DisplayName: "Test Plugin",
			Version:     "v0.1.0",
			Description: "A test plugin",
			Author:      "test-author",
			License:     "Apache-2.0",
			Icon:        "icon.svg",
			URLs: pluginsdk.PluginURLs{
				Homepage:      "https://example.com",
				Repository:    "https://github.com/example/test-plugin",
				Documentation: "https://docs.example.com",
			},
			Tags: []string{"test", "example"},
		},
		Permissions: pluginsdk.Permissions{
			Capabilities: []string{"internet_access", "cluster_scoped_resources"},
			RBAC: []pluginsdk.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		},
		Menu: pluginsdk.MenuDefinition{
			Organization: []pluginsdk.MenuEntry{
				{CRD: "TestResource", List: true, Detail: true, Create: true, Icon: "puzzle"},
			},
		},
		CustomComponents: map[string]pluginsdk.ComponentMapping{
			"TestResource": {List: "TestResourceList", Detail: "TestResourceDetail"},
		},
		UIHints: map[string]pluginsdk.UIHint{
			"TestResource": {
				FormGroups: []pluginsdk.FormGroup{
					{Name: "General", Fields: []string{"name", "namespace"}},
				},
				StatusMapping: pluginsdk.StatusMapping{
					JSONPath: ".status.phase",
					Values: map[string]pluginsdk.StatusValue{
						"Running": {Badge: "success", Label: "Running"},
					},
				},
			},
		},
		CRDs: []string{
			"apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: testresources.test.example.com",
		},
	}
}

func (p *testPlugin) Start(ctx context.Context, host pluginsdk.Host) error {
	host.ReportStatus(pluginsdk.PluginStatus{
		Phase:   pluginsdk.PhaseInstalling,
		Message: "installing test resources",
	})
	host.ReportReady()
	host.ReportStatus(pluginsdk.PluginStatus{
		Phase:   pluginsdk.PhaseRunning,
		Message: "running",
	})
	close(p.started)
	<-ctx.Done()
	return nil
}

func (p *testPlugin) Shutdown(_ context.Context) error {
	close(p.shutdown)
	return nil
}

func TestRunInProcess(t *testing.T) {
	plugin := newTestPlugin()

	ip, err := RunInProcess(plugin)
	if err != nil {
		t.Fatalf("RunInProcess failed: %v", err)
	}

	// Wait for plugin to start
	select {
	case <-plugin.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for plugin to start")
	}

	// Test GetStatus
	statusResp, err := ip.MetadataClient.GetStatus(context.Background(), connect.NewRequest(&pb.GetStatusRequest{}))
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if statusResp.Msg.GetPhase() != string(pluginsdk.PhaseRunning) {
		t.Fatalf("expected phase %q, got %q", pluginsdk.PhaseRunning, statusResp.Msg.GetPhase())
	}
	if statusResp.Msg.GetVersion() != "v0.1.0" {
		t.Fatalf("expected version 'v0.1.0', got %q", statusResp.Msg.GetVersion())
	}

	// Test GetDefinition
	defResp, err := ip.MetadataClient.GetDefinition(context.Background(), connect.NewRequest(&pb.GetDefinitionRequest{}))
	if err != nil {
		t.Fatalf("GetDefinition failed: %v", err)
	}

	if defResp.Msg.GetName() != "test-plugin" {
		t.Fatalf("expected name 'test-plugin', got %q", defResp.Msg.GetName())
	}
	if defResp.Msg.GetDisplayName() != "Test Plugin" {
		t.Fatalf("expected display_name 'Test Plugin', got %q", defResp.Msg.GetDisplayName())
	}
	if defResp.Msg.GetDescription() != "A test plugin" {
		t.Fatalf("expected description 'A test plugin', got %q", defResp.Msg.GetDescription())
	}
	if defResp.Msg.GetAuthor() != "test-author" {
		t.Fatalf("expected author 'test-author', got %q", defResp.Msg.GetAuthor())
	}
	if defResp.Msg.GetLicense() != "Apache-2.0" {
		t.Fatalf("expected license 'Apache-2.0', got %q", defResp.Msg.GetLicense())
	}
	if defResp.Msg.GetIcon() != "icon.svg" {
		t.Fatalf("expected icon 'icon.svg', got %q", defResp.Msg.GetIcon())
	}
	if len(defResp.Msg.GetTags()) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(defResp.Msg.GetTags()))
	}

	// Verify URLs
	urls := defResp.Msg.GetUrls()
	if urls.GetHomepage() != "https://example.com" {
		t.Fatalf("expected homepage 'https://example.com', got %q", urls.GetHomepage())
	}
	if urls.GetRepository() != "https://github.com/example/test-plugin" {
		t.Fatalf("expected repository 'https://github.com/example/test-plugin', got %q", urls.GetRepository())
	}
	if urls.GetDocumentation() != "https://docs.example.com" {
		t.Fatalf("expected documentation 'https://docs.example.com', got %q", urls.GetDocumentation())
	}

	// Verify permissions
	perms := defResp.Msg.GetPermissions()
	if len(perms.GetCapabilities()) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(perms.GetCapabilities()))
	}
	if perms.GetCapabilities()[0] != "internet_access" {
		t.Fatalf("expected capability 'internet_access', got %q", perms.GetCapabilities()[0])
	}
	if len(perms.GetRbac()) != 1 {
		t.Fatalf("expected 1 RBAC rule, got %d", len(perms.GetRbac()))
	}
	rbac := perms.GetRbac()[0]
	if len(rbac.GetResources()) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(rbac.GetResources()))
	}

	// Verify CRDs as YAML strings
	if len(defResp.Msg.GetCrds()) != 1 {
		t.Fatalf("expected 1 CRD, got %d", len(defResp.Msg.GetCrds()))
	}

	// Verify organization menu
	if len(defResp.Msg.GetMenu().GetOrganization()) != 1 {
		t.Fatalf("expected 1 organization menu entry, got %d", len(defResp.Msg.GetMenu().GetOrganization()))
	}
	orgEntry := defResp.Msg.GetMenu().GetOrganization()[0]
	if orgEntry.GetCrd() != "TestResource" {
		t.Fatalf("expected menu CRD 'TestResource', got %q", orgEntry.GetCrd())
	}
	if !orgEntry.GetCreate() {
		t.Fatal("expected menu entry create to be true")
	}
	if orgEntry.GetIcon() != "puzzle" {
		t.Fatalf("expected menu entry icon 'puzzle', got %q", orgEntry.GetIcon())
	}

	// Verify custom components
	cc := defResp.Msg.GetCustomComponents()
	if len(cc) != 1 {
		t.Fatalf("expected 1 custom component, got %d", len(cc))
	}
	if cc["TestResource"].GetList() != "TestResourceList" {
		t.Fatalf("expected custom component list 'TestResourceList', got %q", cc["TestResource"].GetList())
	}

	// Verify UI hints
	hints := defResp.Msg.GetUiHints()
	if len(hints) != 1 {
		t.Fatalf("expected 1 UI hint, got %d", len(hints))
	}
	hint := hints["TestResource"]
	if len(hint.GetFormGroups()) != 1 {
		t.Fatalf("expected 1 form group, got %d", len(hint.GetFormGroups()))
	}
	if hint.GetFormGroups()[0].GetName() != "General" {
		t.Fatalf("expected form group name 'General', got %q", hint.GetFormGroups()[0].GetName())
	}
	if hint.GetStatusMapping().GetJsonPath() != ".status.phase" {
		t.Fatalf("expected status mapping json_path '.status.phase', got %q", hint.GetStatusMapping().GetJsonPath())
	}

	// Stop
	if err := ip.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify shutdown was called
	select {
	case <-plugin.shutdown:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for plugin shutdown")
	}
}

func TestMockHost(t *testing.T) {
	h := NewMockHost()

	if h.IsReady() {
		t.Fatal("expected not ready initially")
	}

	h.ReportStatus(pluginsdk.PluginStatus{
		Phase:   pluginsdk.PhaseInstalling,
		Message: "installing",
	})

	h.ReportReady()
	if !h.IsReady() {
		t.Fatal("expected ready after ReportReady")
	}

	h.ReportStatus(pluginsdk.PluginStatus{
		Phase:   pluginsdk.PhaseRunning,
		Message: "running",
	})

	if len(h.StatusHistory) != 2 {
		t.Fatalf("expected 2 status updates, got %d", len(h.StatusHistory))
	}
	if h.StatusHistory[0].Phase != pluginsdk.PhaseInstalling {
		t.Fatalf("expected first status phase 'installing', got %q", h.StatusHistory[0].Phase)
	}
	if h.StatusHistory[1].Phase != pluginsdk.PhaseRunning {
		t.Fatalf("expected second status phase 'running', got %q", h.StatusHistory[1].Phase)
	}

	status := h.CurrentStatus()
	if status.Phase != pluginsdk.PhaseRunning {
		t.Fatalf("expected current phase 'running', got %q", status.Phase)
	}
}
