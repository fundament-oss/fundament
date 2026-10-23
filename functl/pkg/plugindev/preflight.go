package plugindev

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// CheckError is a startup-check failure with the copy shown to the user.
type CheckError struct {
	Problem string
	Fix     string
	DocsURL string
}

func (e *CheckError) Error() string {
	var b strings.Builder
	b.WriteString(e.Problem)
	if e.Fix != "" {
		b.WriteString("\n")
		b.WriteString(e.Fix)
	}
	b.WriteString("\nSee ")
	b.WriteString(e.DocsURL)
	return b.String()
}

// Cluster is the slice of the sandbox cluster the dev loop inspects.
type Cluster interface {
	Reachable(ctx context.Context) error
	CRDHasDevField(ctx context.Context) (bool, error)
	ControllerEnv(ctx context.Context) (map[string]string, error)
	Installation(ctx context.Context, name string) (state InstallationState, found bool, err error)
	DeleteInstallation(ctx context.Context, name string) error
}

// InstallationState is what the dev loop needs to know about a PluginInstallation.
type InstallationState struct {
	Version string
	// Terminating is true while the controller finalizer still holds a deleted installation.
	Terminating bool
}

const deployFix = "run `just plugins deploy` from a fundament checkout"

// DockerInfo reports whether the docker daemon is reachable.
func DockerInfo() error {
	return exec.Command("docker", "info").Run()
}

// CheckTools verifies the required local tools exist and docker is running.
func CheckTools(l Layout, lookPath func(string) (string, error), dockerInfo func() error) error {
	tools := []struct{ name, url string }{
		{"docker", "https://docker.com"},
		{"skaffold", "https://skaffold.dev"},
	}
	if l.Vite {
		tools = append(tools, struct{ name, url string }{"bun", "https://bun.sh"})
	}
	for _, tool := range tools {
		if _, err := lookPath(tool.name); err != nil {
			return &CheckError{
				Problem: tool.name + " is not installed or not on PATH",
				Fix:     "install it from " + tool.url,
				DocsURL: Sandbox.DocsURL,
			}
		}
	}
	if err := dockerInfo(); err != nil {
		return &CheckError{
			Problem: "the docker daemon is not reachable",
			Fix:     "start docker and retry",
			DocsURL: Sandbox.DocsURL,
		}
	}
	return nil
}

// CheckCluster verifies the sandbox supports dev mode for functlOrgID.
func CheckCluster(ctx context.Context, c Cluster, t Target, functlOrgID string) error {
	fail := func(problem, fix string) error {
		return &CheckError{Problem: problem, Fix: fix, DocsURL: t.DocsURL}
	}

	if err := c.Reachable(ctx); err != nil {
		return fail(fmt.Sprintf("cannot reach kube context %q: %v", t.KubeContext, err),
			"check that the sandbox cluster is running")
	}

	hasDev, err := c.CRDHasDevField(ctx)
	if err != nil {
		return fmt.Errorf("inspect PluginInstallation CRD: %w", err)
	}
	if !hasDev {
		return fail("the PluginInstallation CRD is missing or has no spec.dev field (not installed, or the cluster is too old)", deployFix)
	}

	env, err := c.ControllerEnv(ctx)
	if err != nil {
		return err
	}
	if env["PLUGIN_CONTROLLER_ALLOW_DEV_MODE"] != "true" {
		return fail("the plugin controller does not allow dev mode", deployFix)
	}
	// Dev installations carry no definitionHash.
	if env["PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH"] != "true" {
		return fail("the plugin controller does not allow unpinned definition hashes", deployFix)
	}

	if got := env["FUNDAMENT_ORGANIZATION_ID"]; got != functlOrgID {
		return fail(
			fmt.Sprintf("the plugin controller serves organization %q, but functl is set to %q", got, functlOrgID),
			fmt.Sprintf("run `functl org set %s`", got),
		)
	}
	return nil
}

// CheckVersionBump rejects redeploying an installation under a different version.
func CheckVersionBump(ctx context.Context, c Cluster, installation, version string) error {
	state, found, err := c.Installation(ctx, installation)
	if err != nil {
		return fmt.Errorf("read installation %q: %w", installation, err)
	}
	// Installed versions come from the registry, which drops a leading "v".
	version = strings.TrimPrefix(version, "v")
	current := state.Version
	if !found || current == version {
		return nil
	}
	return &CheckError{
		Problem: fmt.Sprintf("installation %q runs version %s but the plugin is now %s", installation, current, version),
		Fix:     "run `functl plugin dev-down` first",
		DocsURL: Sandbox.DocsURL,
	}
}

// Teardown polling defaults; the controller finalizer needs time to clean up.
const (
	TeardownPollInterval = time.Second
	TeardownTimeout      = 2 * time.Minute
)

// WaitGone polls until the installation is NotFound, printing one waiting line to out.
func WaitGone(ctx context.Context, c Cluster, name string, out io.Writer, interval, timeout time.Duration) error {
	fmt.Fprintf(out, "waiting for the controller to tear down %s…\n", name)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		_, found, err := c.Installation(ctx, name)
		if err != nil {
			return fmt.Errorf("read installation %q: %w", name, err)
		}
		if !found {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out after %s waiting for the controller to tear down %s; check the plugin controller logs", timeout, name)
		case <-tick.C:
		}
	}
}

// WaitIfTerminating waits for an installation that is already being deleted; otherwise it does nothing.
func WaitIfTerminating(ctx context.Context, c Cluster, name string, out io.Writer, interval, timeout time.Duration) error {
	state, found, err := c.Installation(ctx, name)
	if err != nil {
		return fmt.Errorf("read installation %q: %w", name, err)
	}
	if !found || !state.Terminating {
		return nil
	}
	return WaitGone(ctx, c, name, out, interval, timeout)
}

// DevDown deletes the installation and waits for the controller to finish tearing it down.
func DevDown(ctx context.Context, c Cluster, name string, out io.Writer, interval, timeout time.Duration) error {
	_, found, err := c.Installation(ctx, name)
	if err != nil {
		return fmt.Errorf("read installation %q: %w", name, err)
	}
	if !found {
		fmt.Fprintf(out, "PluginInstallation %s not found; nothing to remove\n", name)
		return nil
	}
	if err := c.DeleteInstallation(ctx, name); err != nil {
		return err
	}
	if err := WaitGone(ctx, c, name, out, interval, timeout); err != nil {
		return err
	}
	fmt.Fprintf(out, "PluginInstallation %s is gone\n", name)
	return nil
}

// CheckCleanedUp reports an installation that skaffold's cleanup left in place.
func CheckCleanedUp(ctx context.Context, c Cluster, name string) error {
	state, found, err := c.Installation(ctx, name)
	if err != nil {
		return fmt.Errorf("verify cleanup of %q: %w", name, err)
	}
	if found && !state.Terminating {
		return fmt.Errorf("PluginInstallation %s was not removed on exit; run `functl plugin dev-down`", name)
	}
	return nil
}
