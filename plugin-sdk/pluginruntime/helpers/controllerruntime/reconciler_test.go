package controllerruntime

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"
)

// controller-runtime drops every log line until SetLogger is called, and a
// reconciler's only way to explain why it could not make progress is
// log.FromContext. Without this wiring that output goes nowhere.
func TestSetDefaultLoggerRoutesIntoSlog(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	setDefaultLogger()
	ctrl.Log.Info("reconciler-said-something", "pool", "test-pool")

	assert.Contains(t, buf.String(), "reconciler-said-something")
	assert.Contains(t, buf.String(), "test-pool")
}
