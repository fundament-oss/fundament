package sdktesting

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pb "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
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

func (p *testPlugin) Definition() pluginruntime.PluginDefinition {
	return pluginruntime.PluginDefinition{
		Metadata: pluginruntime.PluginMetadata{
			Name:        "test-plugin",
			DisplayName: "Test Plugin",
			Version:     "v0.1.0",
			Description: "A test plugin",
			Author:      "test-author",
			License:     "Apache-2.0",
			Icon:        "icon.svg",
			URLs: pluginruntime.PluginURLs{
				Homepage:      "https://example.com",
				Repository:    "https://github.com/example/test-plugin",
				Documentation: "https://docs.example.com",
			},
			Tags: []string{"test", "example"},
		},
		Permissions: pluginruntime.Permissions{
			Capabilities: []string{"internet_access", "cluster_scoped_resources"},
			RBAC: []pluginruntime.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		},
		Menu: pluginruntime.MenuDefinition{
			Organization: []pluginruntime.MenuEntry{
				{CRD: "TestResource", List: true, Detail: true, Create: true, Icon: "puzzle"},
			},
		},
		CustomComponents: map[string]pluginruntime.ComponentMapping{
			"TestResource": {List: "TestResourceList", Detail: "TestResourceDetail"},
		},
		UIHints: map[string]pluginruntime.UIHint{
			"TestResource": {
				FormGroups: []pluginruntime.FormGroup{
					{Name: "General", Fields: []string{"name", "namespace"}},
				},
				StatusMapping: pluginruntime.StatusMapping{
					JSONPath: ".status.phase",
					Values: map[string]pluginruntime.StatusValue{
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

func (p *testPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
	host.ReportStatus(pluginruntime.PluginStatus{
		Phase:   pluginruntime.PhaseInstalling,
		Message: "installing test resources",
	})
	host.ReportReady()
	host.ReportStatus(pluginruntime.PluginStatus{
		Phase:   pluginruntime.PhaseRunning,
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
	require.NoError(t, err)

	// Wait for plugin to start
	select {
	case <-plugin.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for plugin to start")
	}

	// Test GetStatus
	statusResp, err := ip.MetadataClient.GetStatus(context.Background(), connect.NewRequest(&pb.GetStatusRequest{}))
	require.NoError(t, err)
	assert.Equal(t, string(pluginruntime.PhaseRunning), statusResp.Msg.GetPhase())
	assert.Equal(t, "v0.1.0", statusResp.Msg.GetVersion())

	// Test GetDefinition
	defResp, err := ip.MetadataClient.GetDefinition(context.Background(), connect.NewRequest(&pb.GetDefinitionRequest{}))
	require.NoError(t, err)

	assert.Equal(t, "test-plugin", defResp.Msg.GetName())
	assert.Equal(t, "Test Plugin", defResp.Msg.GetDisplayName())
	assert.Equal(t, "A test plugin", defResp.Msg.GetDescription())
	assert.Equal(t, "test-author", defResp.Msg.GetAuthor())
	assert.Equal(t, "Apache-2.0", defResp.Msg.GetLicense())
	assert.Equal(t, "icon.svg", defResp.Msg.GetIcon())
	assert.Len(t, defResp.Msg.GetTags(), 2)

	// Verify URLs
	urls := defResp.Msg.GetUrls()
	assert.Equal(t, "https://example.com", urls.GetHomepage())
	assert.Equal(t, "https://github.com/example/test-plugin", urls.GetRepository())
	assert.Equal(t, "https://docs.example.com", urls.GetDocumentation())

	// Verify permissions
	perms := defResp.Msg.GetPermissions()
	require.Len(t, perms.GetCapabilities(), 2)
	assert.Equal(t, "internet_access", perms.GetCapabilities()[0])
	require.Len(t, perms.GetRbac(), 1)
	assert.Len(t, perms.GetRbac()[0].GetResources(), 2)

	// Verify CRDs as YAML strings
	assert.Len(t, defResp.Msg.GetCrds(), 1)

	// Verify organization menu
	require.Len(t, defResp.Msg.GetMenu().GetOrganization(), 1)
	orgEntry := defResp.Msg.GetMenu().GetOrganization()[0]
	assert.Equal(t, "TestResource", orgEntry.GetCrd())
	assert.True(t, orgEntry.GetCreate())
	assert.Equal(t, "puzzle", orgEntry.GetIcon())

	// Verify custom components
	cc := defResp.Msg.GetCustomComponents()
	require.Len(t, cc, 1)
	assert.Equal(t, "TestResourceList", cc["TestResource"].GetList())

	// Verify UI hints
	hints := defResp.Msg.GetUiHints()
	require.Len(t, hints, 1)
	hint := hints["TestResource"]
	require.Len(t, hint.GetFormGroups(), 1)
	assert.Equal(t, "General", hint.GetFormGroups()[0].GetName())
	assert.Equal(t, ".status.phase", hint.GetStatusMapping().GetJsonPath())

	// Stop
	require.NoError(t, ip.Stop())

	// Verify shutdown was called
	select {
	case <-plugin.shutdown:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for plugin shutdown")
	}
}

func TestMockHost(t *testing.T) {
	h := NewMockHost()

	assert.False(t, h.IsReady())

	h.ReportStatus(pluginruntime.PluginStatus{
		Phase:   pluginruntime.PhaseInstalling,
		Message: "installing",
	})

	h.ReportReady()
	assert.True(t, h.IsReady())

	h.ReportStatus(pluginruntime.PluginStatus{
		Phase:   pluginruntime.PhaseRunning,
		Message: "running",
	})

	require.Len(t, h.StatusHistory, 2)
	assert.Equal(t, pluginruntime.PhaseInstalling, h.StatusHistory[0].Phase)
	assert.Equal(t, pluginruntime.PhaseRunning, h.StatusHistory[1].Phase)
	assert.Equal(t, pluginruntime.PhaseRunning, h.CurrentStatus().Phase)
}
