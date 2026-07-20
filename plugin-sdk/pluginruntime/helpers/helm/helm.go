// Package helm provides optional helpers for managing Helm releases from plugins.
// It shells out to the helm CLI, which must be available in the container's PATH.
package helm

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// BANDAID (FUN-17): plugin-controller materialises the plugin SA's scope
// ClusterRole from this plugin's GetDefinition, which it can only read once the
// pod is running — so the very first `helm install` races that grant and fails
// with "secrets is forbidden". Retrying on RBAC-forbidden errors keeps the pod
// (and its metadata/GetDefinition server) alive until the controller catches up,
// after which a retry succeeds. Remove once the plugin definition moves out of
// the plugin container so the RBAC can be granted before the pod starts.
const (
	rbacRetryTimeout  = 3 * time.Minute
	rbacRetryInterval = 3 * time.Second
)

// Client wraps Helm CLI operations for a given namespace.
type Client struct {
	namespace string
}

// NewClient creates a Helm client that operates in the given namespace.
func NewClient(namespace string) *Client {
	return &Client{namespace: namespace}
}

// Install runs "helm upgrade --install" for the given release and chart.
func (c *Client) Install(ctx context.Context, releaseName, chart string, values map[string]string) error {
	args := []string{"upgrade", "--install", releaseName, chart, "--namespace", c.namespace, "--create-namespace", "--wait"}
	args = appendSortedValues(args, values)
	return c.runInstall(ctx, args)
}

// InstallFromRepo runs "helm upgrade --install" for a chart from a remote repository.
// If version is non-empty, it pins the chart to that version via --version.
func (c *Client) InstallFromRepo(ctx context.Context, releaseName, chart, repoURL, version string, values map[string]string) error {
	args := []string{"upgrade", "--install", releaseName, chart, "--repo", repoURL, "--namespace", c.namespace, "--create-namespace", "--wait"}
	if version != "" {
		args = append(args, "--version", version)
	}
	args = appendSortedValues(args, values)
	return c.runInstall(ctx, args)
}

// runInstall executes a helm install, retrying on RBAC-forbidden errors until
// the plugin SA's scope is granted (see the BANDAID note above). Any other
// failure returns immediately.
func (c *Client) runInstall(ctx context.Context, args []string) error {
	deadline := time.Now().Add(rbacRetryTimeout)
	for {
		cmd := exec.CommandContext(ctx, "helm", args...) //nolint:gosec // args are constructed internally
		output, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		out := strings.TrimSpace(string(output))
		if !isRBACForbidden(out) || time.Now().After(deadline) {
			return fmt.Errorf("helm install failed: %s: %w", out, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("helm install cancelled while awaiting plugin RBAC: %w", ctx.Err())
		case <-time.After(rbacRetryInterval):
		}
	}
}

// isRBACForbidden reports whether helm output indicates the plugin SA is not yet
// authorised — i.e. the controller hasn't materialised the scope ClusterRole yet.
func isRBACForbidden(output string) bool {
	return strings.Contains(output, "is forbidden")
}

// IsInstalled checks whether a Helm release exists in the client's namespace.
func (c *Client) IsInstalled(ctx context.Context, releaseName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "helm", "status", releaseName, "--namespace", c.namespace) //nolint:gosec // args are constructed internally
	if err := cmd.Run(); err != nil {
		return false, nil //nolint:nilerr // non-zero exit means release not found, not an error
	}
	return true, nil
}

func appendSortedValues(args []string, values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--set", k+"="+values[k])
	}
	return args
}

// Uninstall runs "helm uninstall" for the given release.
func (c *Client) Uninstall(ctx context.Context, releaseName string) error {
	cmd := exec.CommandContext(ctx, "helm", "uninstall", releaseName, "--namespace", c.namespace) //nolint:gosec // args are constructed internally
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm uninstall failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
