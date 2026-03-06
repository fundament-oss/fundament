// Package helm provides optional helpers for managing Helm releases from plugins.
package helm

import (
	"context"
	"fmt"
	"os/exec"
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
	for k, v := range values {
		args = append(args, "--set", k+"="+v)
	}
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// InstallFromRepo runs "helm upgrade --install" for a chart from a remote repository.
func (c *Client) InstallFromRepo(ctx context.Context, releaseName, chart, repoURL string, values map[string]string) error {
	args := []string{"upgrade", "--install", releaseName, chart, "--repo", repoURL, "--namespace", c.namespace, "--create-namespace", "--wait"}
	for k, v := range values {
		args = append(args, "--set", k+"="+v)
	}
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// Uninstall runs "helm uninstall" for the given release.
func (c *Client) Uninstall(ctx context.Context, releaseName string) error {
	cmd := exec.CommandContext(ctx, "helm", "uninstall", releaseName, "--namespace", c.namespace)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm uninstall failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
