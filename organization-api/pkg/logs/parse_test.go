package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Gardener's system components log in klog, and per-shoot Vali holds exactly
// those streams. Without a klog header parse every system line was classified as
// the default level: the ERROR chip read zero and a severity filter for ERROR
// matched nothing, on the only data the backend actually serves.
func TestParseLogLineKlogSeverity(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantLevel string
	}{
		{"error", "E0804 12:33:01.123456       1 reflector.go:1] failed to sync", "ERROR"},
		{"fatal", "F0804 12:33:01.123456       1 main.go:9] cannot continue", "ERROR"},
		{"warning", "W0804 12:33:01.123456       1 shared.go:2] retrying", "WARN"},
		{"info", "I0804 12:33:01.123456       1 server.go:3] serving", "INFO"},
		// Prose that merely starts with a capital letter must not be read as a
		// severity: the header shape is checked, not just the first byte.
		{"prose is not a severity", "Error connecting to database", ""},
		{"wrong shape", "E080 12:33:01 not klog", ""},
		{"too short", "E0804", ""},
		{"unknown letter", "X0804 12:33:01.123456       1 x.go:1] hi", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, level, _ := parseLogLine(tt.line)
			assert.Equal(t, tt.wantLevel, level)
		})
	}
}

// A JSON severity field still wins; klog parsing is only the non-JSON path.
func TestParseLogLineJSONStillWins(t *testing.T) {
	_, level, fields := parseLogLine(`{"level":"warn","msg":"careful"}`)
	assert.Equal(t, "warn", level)
	assert.Equal(t, "careful", fields["msg"])
}
