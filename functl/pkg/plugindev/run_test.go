package plugindev

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDigestRef(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)

	got, err := digestRef("localhost:5112/demo-plugin-dev:abc@" + digest)
	require.NoError(t, err)
	assert.Equal(t, "localhost:5112/demo-plugin-dev@"+digest, got)

	_, err = digestRef("localhost:5112/demo-plugin-dev:abc")
	require.Error(t, err)
}

func TestSuperviseStopsSiblings(t *testing.T) {
	sleeper := exec.Command("sleep", "30")
	failer := exec.Command("sh", "-c", "exit 3")

	start := time.Now()
	err := supervise(context.Background(), context.Background(), sleeper, failer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 3")
	assert.Less(t, time.Since(start), 15*time.Second)

	// A reaped, signalled sleep is gone rather than left running for 30s.
	require.NotNil(t, sleeper.ProcessState, "sleep was not waited for")
	assert.False(t, sleeper.ProcessState.Success())
}

func TestSuperviseContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sleeper := exec.Command("sleep", "30")

	done := make(chan error, 1)
	go func() { done <- supervise(ctx, context.Background(), sleeper) }()
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("supervise did not return after cancel")
	}
}

func TestLineWriterPrefixesWholeLines(t *testing.T) {
	var out strings.Builder
	var mu sync.Mutex
	w := &lineWriter{prefix: []byte("[vite] "), out: &out, mu: &mu}

	_, _ = w.Write([]byte("hel"))
	assert.Empty(t, out.String())
	_, _ = w.Write([]byte("lo\nwor"))
	_, _ = w.Write([]byte("ld\nend"))
	w.flush()

	assert.Equal(t, "[vite] hello\n[vite] world\n[vite] end\n", out.String())
}

func TestSuperviseCleanExitIsAnError(t *testing.T) {
	err := supervise(context.Background(), context.Background(), exec.Command("true"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exited unexpectedly")
}

func TestLineWriterFlushesLongPartialLine(t *testing.T) {
	var out strings.Builder
	var mu sync.Mutex
	w := &lineWriter{prefix: []byte("> "), out: &out, mu: &mu}

	_, _ = w.Write([]byte(strings.Repeat("x", maxPartialLine)))
	assert.Equal(t, "> "+strings.Repeat("x", maxPartialLine)+"\n", out.String())
	assert.Empty(t, w.buf)
}

func TestSuperviseForceKillsWithoutGrace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	force, forceCancel := context.WithCancel(context.Background())
	// Ignores SIGINT, so only a kill stops it before stopGrace.
	stubborn := exec.Command("sh", "-c", "trap '' INT; sleep 30")

	done := make(chan error, 1)
	go func() { done <- supervise(ctx, force, stubborn) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	forceCancel()

	select {
	case <-done:
		assert.Less(t, time.Since(start), stopGrace/2)
	case <-time.After(stopGrace + 5*time.Second):
		t.Fatal("supervise did not return after force")
	}
}
