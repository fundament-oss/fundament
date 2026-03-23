// Package helm provides optional helpers for managing Helm releases from plugins.
// It shells out to the helm CLI, which must be available in the container's PATH.
package helm

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
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
	cmd := exec.CommandContext(ctx, "helm", args...) //nolint:gosec // args are constructed internally
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// InstallFromRepo runs "helm upgrade --install" for a chart from a remote repository.
// If version is non-empty, it pins the chart to that version via --version.
func (c *Client) InstallFromRepo(ctx context.Context, releaseName, chart, repoURL, version string, values map[string]string) error {
	args := []string{"upgrade", "--install", releaseName, chart, "--repo", repoURL, "--namespace", c.namespace, "--create-namespace", "--wait"}
	if version != "" {
		args = append(args, "--version", version)
	}
	args = appendSortedValues(args, values)
	cmd := exec.CommandContext(ctx, "helm", args...) //nolint:gosec // args are constructed internally
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
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
