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
