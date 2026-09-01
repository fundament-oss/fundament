package pluginmachinery

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestConfigEnabled(t *testing.T) {
	t.Parallel()
	assert.False(t, Config{}.Enabled())
	assert.False(t, Config{ControllerImage: "img"}.Enabled())
	assert.False(t, Config{OrganizationAPIURL: "url"}.Enabled())
	assert.True(t, Config{ControllerImage: "img", OrganizationAPIURL: "url"}.Enabled())
}

func TestSyncRejectsUnexpectedEntity(t *testing.T) {
	t.Parallel()
	h := &Handler{
		shoot:  shoot.NewMockShootAccess(testLogger()),
		cfg:    Config{ControllerImage: "img", OrganizationAPIURL: "url"},
		logger: testLogger(),
	}

	err := h.Sync(context.Background(), uuid.New(), handler.SyncContext{EntityType: handler.EntityOrgUser})
	require.Error(t, err)
}

// With no image/URL configured the handler must no-op without touching the
// database or the shoot — mock-gardener and PR environments have neither.
func TestDisabledConfigNoOps(t *testing.T) {
	t.Parallel()
	mock := shoot.NewMockShootAccess(testLogger())
	// queries deliberately nil: a disabled handler must return before any DB use.
	h := &Handler{shoot: mock, cfg: Config{}, logger: testLogger()}

	err := h.Sync(context.Background(), uuid.New(), handler.SyncContext{EntityType: handler.EntityCluster})
	require.NoError(t, err)
	err = h.Reconcile(context.Background())
	require.NoError(t, err)

	assert.Empty(t, mock.CRDs)
	assert.Empty(t, mock.Deployments)
}
