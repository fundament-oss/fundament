package plugindev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// stopGrace is how long children get to exit after SIGINT; skaffold uses it to clean up.
const stopGrace = 10 * time.Second

// BuildConsole runs the initial Vite build so the first image has console assets.
func BuildConsole(ctx context.Context, l Layout) error {
	if !l.Vite {
		return nil
	}
	dir := filepath.Join(l.PluginDir, "console-ui")
	for _, args := range [][]string{{"install"}, {"run", "build"}} {
		cmd := exec.CommandContext(ctx, "bun", args...)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("bun %s in %s: %w", strings.Join(args, " "), dir, err)
		}
	}
	return nil
}

// BuildImage builds and pushes the dev image and returns its repo@sha256 reference.
func BuildImage(ctx context.Context, renderDir string, t Target) (string, error) {
	cmd := exec.CommandContext(ctx, "skaffold", "build",
		"-f", "skaffold.yaml",
		"--default-repo", t.Registry,
		"--file-output", "build.json",
	)
	cmd.Dir = renderDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("skaffold build: %w", err)
	}

	raw, err := os.ReadFile(filepath.Join(renderDir, "build.json"))
	if err != nil {
		return "", fmt.Errorf("read skaffold build output: %w", err)
	}
	var out struct {
		Builds []struct {
			ImageName string `json:"imageName"`
			Tag       string `json:"tag"`
		} `json:"builds"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("parse skaffold build output: %w", err)
	}
	if len(out.Builds) == 0 {
		return "", errors.New("skaffold build reported no built images")
	}
	return digestRef(out.Builds[0].Tag)
}

// digestRef turns repo:tag@sha256:… into repo@sha256:…, dropping the tag.
func digestRef(tag string) (string, error) {
	ref, digest, ok := strings.Cut(tag, "@")
	if !ok || !strings.HasPrefix(digest, "sha256:") {
		return "", fmt.Errorf("image %q has no sha256 digest", tag)
	}
	// A colon after the last slash separates the tag; earlier ones are a registry port.
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref + "@" + digest, nil
}

// Dev runs the Vite watcher (for Vite plugins) and skaffold dev until either exits or ctx is done.
func Dev(ctx context.Context, renderDir string, l Layout, t Target, namespace string, cleanup bool) error {
	var mu sync.Mutex
	var cmds []*exec.Cmd

	if l.Vite {
		vite := exec.Command("bunx", "vite", "build", "--watch", "--emptyOutDir=false")
		vite.Dir = filepath.Join(l.PluginDir, "console-ui")
		prefixOutput(vite, "[vite] ", &mu)
		cmds = append(cmds, vite)
	}

	skaffold := exec.Command("skaffold", "dev",
		"-f", "skaffold.yaml",
		"--default-repo", t.Registry,
		"-n", namespace,
		"--status-check=false",
		"--cleanup="+strconv.FormatBool(cleanup),
	)
	skaffold.Dir = renderDir
	prefixOutput(skaffold, "[skaffold] ", &mu)
	cmds = append(cmds, skaffold)

	return supervise(ctx, cmds...)
}

type exitResult struct {
	cmd *exec.Cmd
	err error
}

// supervise starts cmds, and once one exits or ctx is done stops the rest and waits for all.
// It returns the first child's error; a ctx cancellation counts as a clean stop and returns nil.
func supervise(ctx context.Context, cmds ...*exec.Cmd) error {
	results := make(chan exitResult, len(cmds))
	running := map[*exec.Cmd]bool{}

	var startErr error
	for _, cmd := range cmds {
		setProcessGroup(cmd)
		cmd.WaitDelay = stopGrace
		if err := cmd.Start(); err != nil {
			startErr = fmt.Errorf("start %s: %w", cmd.Path, err)
			break
		}
		running[cmd] = true
		go func() {
			err := cmd.Wait()
			if f, ok := cmd.Stdout.(interface{ flush() }); ok {
				f.flush()
			}
			results <- exitResult{cmd: cmd, err: err}
		}()
	}

	firstErr := startErr
	stopping := startErr != nil
	var killTimer <-chan time.Time
	stop := func() {
		stopping = true
		for cmd := range running {
			interruptGroup(cmd)
		}
		killTimer = time.After(stopGrace)
	}
	if stopping {
		stop()
	}

	done := ctx.Done()
	for len(running) > 0 {
		select {
		case <-done:
			done = nil
			if !stopping {
				stop()
			}
		case <-killTimer:
			for cmd := range running {
				killGroup(cmd)
			}
		case r := <-results:
			delete(running, r.cmd)
			if stopping {
				continue
			}
			name := filepath.Base(r.cmd.Path)
			announceExit(r.cmd, name)
			switch {
			case r.err != nil:
				firstErr = fmt.Errorf("%s exited: %w", name, r.err)
			case ctx.Err() == nil:
				firstErr = fmt.Errorf("%s exited unexpectedly", name)
			}
			stop()
		}
	}
	return firstErr
}

// announceExit reports a child's exit through its prefixed output, if it has one.
func announceExit(cmd *exec.Cmd, name string) {
	msg := name + " exited; stopping\n"
	if w, ok := cmd.Stdout.(*lineWriter); ok {
		_, _ = w.Write([]byte(msg))
		return
	}
	fmt.Fprint(os.Stdout, msg)
}

// prefixOutput sends cmd's stdout and stderr through one line-buffered, prefixed writer.
func prefixOutput(cmd *exec.Cmd, prefix string, mu *sync.Mutex) {
	w := &lineWriter{prefix: []byte(prefix), out: os.Stdout, mu: mu}
	// The same writer for both lets exec share one pipe, keeping their lines ordered.
	cmd.Stdout = w
	cmd.Stderr = w
}

// lineWriter writes whole prefixed lines under mu so children don't interleave mid-line.
type lineWriter struct {
	prefix []byte
	out    io.Writer
	mu     *sync.Mutex
	buf    []byte
}

// maxPartialLine caps how much of an unterminated line is held back.
const maxPartialLine = 64 << 10

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if i := bytes.LastIndexByte(w.buf, '\n'); i >= 0 {
		w.emit(w.buf[:i+1])
		w.buf = append(w.buf[:0], w.buf[i+1:]...)
	}
	if len(w.buf) >= maxPartialLine {
		w.flush()
	}
	return len(p), nil
}

// flush writes a trailing partial line once the child has exited.
func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(append(w.buf, '\n'))
		w.buf = w.buf[:0]
	}
}

func (w *lineWriter) emit(lines []byte) {
	var out bytes.Buffer
	for line := range bytes.Lines(lines) {
		out.Write(w.prefix)
		out.Write(line)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.out.Write(out.Bytes())
}
