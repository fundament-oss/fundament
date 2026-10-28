package plugindev

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCluster struct {
	reachableErr error
	crdDev       bool
	env          map[string]string
	version      string
	found        bool
	terminating  bool
	notDev       bool
	// goneAfter is how many Installation calls report the installation before it vanishes; 0 means never.
	goneAfter int
	calls     int
	deleted   bool
}

func (f *fakeCluster) Reachable(context.Context) error { return f.reachableErr }

func (f *fakeCluster) CRDHasDevField(context.Context) (bool, error) { return f.crdDev, nil }

func (f *fakeCluster) ControllerEnv(context.Context) (map[string]string, error) { return f.env, nil }

func (f *fakeCluster) Installation(context.Context, string) (InstallationState, bool, error) {
	f.calls++
	if f.goneAfter > 0 && f.calls > f.goneAfter {
		f.found = false
	}
	return InstallationState{Version: f.version, Dev: !f.notDev, Terminating: f.terminating}, f.found, nil
}

func (f *fakeCluster) DeleteInstallation(context.Context, string) error {
	f.deleted = true
	f.terminating = true
	return nil
}

func okCluster() *fakeCluster {
	return &fakeCluster{
		crdDev: true,
		env: map[string]string{
			"PLUGIN_CONTROLLER_ALLOW_DEV_MODE":      "true",
			"PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH": "true",
			"FUNDAMENT_ORGANIZATION_ID":             "org-1",
		},
	}
}

func lookAll(name string) (string, error) { return "/bin/" + name, nil }

func requireCheckError(t *testing.T, err error, fixContains string) {
	t.Helper()
	var ce *CheckError
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, err.Error(), Sandbox.DocsURL)
	assert.Contains(t, ce.Fix, fixContains)
}

func TestCheckToolsMissingSkaffold(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "skaffold" {
			return "", errors.New("not found")
		}
		return lookAll(name)
	}
	err := CheckTools(Layout{}, look, func() error { return nil })
	requireCheckError(t, err, "skaffold.dev")
}

func TestCheckToolsBunOnlyForVite(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "bun" {
			return "", errors.New("not found")
		}
		return lookAll(name)
	}
	require.NoError(t, CheckTools(Layout{Vite: false}, look, func() error { return nil }))
	err := CheckTools(Layout{Vite: true}, look, func() error { return nil })
	requireCheckError(t, err, "bun.sh")
}

func TestCheckToolsDockerDaemonDown(t *testing.T) {
	err := CheckTools(Layout{}, lookAll, func() error { return errors.New("down") })
	requireCheckError(t, err, "docker")
}

func TestCheckClusterCRDTooOld(t *testing.T) {
	c := okCluster()
	c.crdDev = false
	err := CheckCluster(context.Background(), c, Sandbox, "org-1")
	requireCheckError(t, err, "just plugins deploy")
}

func TestCheckClusterDevModeOff(t *testing.T) {
	c := okCluster()
	c.env["PLUGIN_CONTROLLER_ALLOW_DEV_MODE"] = "false"
	err := CheckCluster(context.Background(), c, Sandbox, "org-1")
	requireCheckError(t, err, "just plugins deploy")
}

func TestCheckClusterOrgMismatch(t *testing.T) {
	err := CheckCluster(context.Background(), okCluster(), Sandbox, "org-2")
	requireCheckError(t, err, "functl org set org-1")
}

func TestCheckClusterUnreachable(t *testing.T) {
	c := okCluster()
	c.reachableErr = errors.New("refused")
	err := CheckCluster(context.Background(), c, Sandbox, "org-1")
	requireCheckError(t, err, "")
}

func TestCheckClusterOK(t *testing.T) {
	require.NoError(t, CheckCluster(context.Background(), okCluster(), Sandbox, "org-1"))
}

func TestCheckVersionBump(t *testing.T) {
	c := okCluster()
	c.found, c.version = true, "1.0.0"
	err := CheckVersionBump(context.Background(), c, "p", "1.1.0")
	requireCheckError(t, err, "functl plugin dev-down")
}

func TestCheckVersionBumpSameOrMissing(t *testing.T) {
	c := okCluster()
	c.found, c.version = true, "1.0.0"
	require.NoError(t, CheckVersionBump(context.Background(), c, "p", "1.0.0"))
	c.found = false
	require.NoError(t, CheckVersionBump(context.Background(), c, "p", "2.0.0"))
}

func TestCheckErrorFormat(t *testing.T) {
	e := &CheckError{Problem: "bad", Fix: "do x", DocsURL: "u"}
	assert.Equal(t, "bad\ndo x\nSee u", e.Error())
	e.Fix = ""
	assert.Equal(t, "bad\nSee u", e.Error())
}

func TestCheckClusterCRDMessageCoversMissing(t *testing.T) {
	c := okCluster()
	c.crdDev = false
	err := CheckCluster(context.Background(), c, Sandbox, "org-1")
	assert.Contains(t, err.Error(), "missing")
}

func TestWaitIfTerminatingWaitsThenProceeds(t *testing.T) {
	c := okCluster()
	c.found, c.terminating, c.goneAfter = true, true, 3
	var out strings.Builder
	require.NoError(t, WaitIfTerminating(context.Background(), c, "p", &out, time.Millisecond, time.Second))
	assert.False(t, c.found)
	assert.Equal(t, 1, strings.Count(out.String(), "waiting for the controller to tear down p"))
}

func TestWaitIfTerminatingSkipsLive(t *testing.T) {
	c := okCluster()
	c.found = true
	var out strings.Builder
	require.NoError(t, WaitIfTerminating(context.Background(), c, "p", &out, time.Millisecond, time.Second))
	assert.Empty(t, out.String())
}

func TestDevDownAbsent(t *testing.T) {
	c := okCluster()
	var out strings.Builder
	require.NoError(t, DevDown(context.Background(), c, "p", &out, time.Millisecond, time.Second))
	assert.False(t, c.deleted)
	assert.Contains(t, out.String(), "nothing to remove")
}

func TestDevDownWaitsForController(t *testing.T) {
	c := okCluster()
	c.found, c.goneAfter = true, 3
	var out strings.Builder
	require.NoError(t, DevDown(context.Background(), c, "p", &out, time.Millisecond, time.Second))
	assert.True(t, c.deleted)
	assert.Contains(t, out.String(), "waiting for the controller to tear down p")
	assert.Contains(t, out.String(), "is gone")
}

func TestDevDownTimeout(t *testing.T) {
	c := okCluster()
	c.found = true
	var out strings.Builder
	err := DevDown(context.Background(), c, "p", &out, time.Millisecond, 20*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestCheckCleanedUp(t *testing.T) {
	c := okCluster()
	require.NoError(t, CheckCleanedUp(context.Background(), c, "p"))

	c.found, c.terminating = true, true
	require.NoError(t, CheckCleanedUp(context.Background(), c, "p"))

	c.terminating = false
	err := CheckCleanedUp(context.Background(), c, "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "functl plugin dev-down")
}

func TestCheckClusterUnpinnedHashOff(t *testing.T) {
	c := okCluster()
	delete(c.env, "PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH")
	err := CheckCluster(context.Background(), c, Sandbox, "org-1")
	requireCheckError(t, err, "just plugins deploy")
	assert.Contains(t, err.Error(), "unpinned")
}

func TestCheckVersionBumpIgnoresLeadingV(t *testing.T) {
	c := okCluster()
	c.found, c.version = true, "1.0.0-dev"
	require.NoError(t, CheckVersionBump(context.Background(), c, "p", "v1.0.0-dev"))
}

func TestCheckVersionBumpRejectsNonDev(t *testing.T) {
	c := okCluster()
	c.found, c.notDev, c.version = true, true, "1.0.0-dev"
	err := CheckVersionBump(context.Background(), c, "p", "1.0.0-dev")
	requireCheckError(t, err, "uninstall it first")
}

func TestDevDownRejectsNonDev(t *testing.T) {
	c := okCluster()
	c.found, c.notDev = true, true
	err := DevDown(context.Background(), c, "p", &strings.Builder{}, time.Millisecond, time.Second)
	requireCheckError(t, err, "uninstall it first")
	assert.False(t, c.deleted)
}

func TestCheckToolsMissingKubectl(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "kubectl" {
			return "", errors.New("not found")
		}
		return "/bin/" + name, nil
	}
	err := CheckTools(Layout{}, look, func() error { return nil })
	requireCheckError(t, err, "kubernetes.io")
}
