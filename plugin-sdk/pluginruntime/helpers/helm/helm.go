// Package helm provides optional helpers for managing Helm releases from plugins.
// It shells out to the helm CLI, which must be available in the container's PATH.
package helm

import (
	"bytes"
	"context"
	"encoding/json"
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

// ociInstallArgs builds the "helm upgrade --install" argument list for an OCI
// chart reference, pinning --version when set. Split out from InstallFromOCI so
// the argument construction is unit-testable without shelling out to helm.
func (c *Client) ociInstallArgs(releaseName, chartRef, version string, values map[string]string) []string {
	args := []string{"upgrade", "--install", releaseName, chartRef, "--namespace", c.namespace, "--create-namespace", "--wait"}
	if version != "" {
		args = append(args, "--version", version)
	}
	return appendSortedValues(args, values)
}

// InstallFromOCI runs "helm upgrade --install" for a chart hosted in an OCI
// registry (chartRef like "oci://docker.io/envoyproxy/gateway-helm"). Unlike
// InstallFromRepo there is no --repo; the version pins the chart via --version.
func (c *Client) InstallFromOCI(ctx context.Context, releaseName, chartRef, version string, values map[string]string) error {
	return c.runInstall(ctx, c.ociInstallArgs(releaseName, chartRef, version, values))
}

// runInstall executes a helm install, retrying on RBAC-forbidden errors until
// the plugin SA's scope is granted (see the BANDAID note above). Any other
// failure returns immediately.
func (c *Client) runInstall(ctx context.Context, args []string) error {
	return retryOnRBACForbidden(ctx, "install", func() (string, error) {
		output, err := exec.CommandContext(ctx, "helm", args...).CombinedOutput() //nolint:gosec // args are constructed internally
		return strings.TrimSpace(string(output)), err
	})
}

// retryOnRBACForbidden runs attempt until it succeeds, fails for a reason other
// than RBAC, or rbacRetryTimeout passes. attempt returns helm's error output.
func retryOnRBACForbidden(ctx context.Context, op string, attempt func() (string, error)) error {
	deadline := time.Now().Add(rbacRetryTimeout)
	for {
		out, err := attempt()
		if err == nil {
			return nil
		}
		if !isRBACForbidden(out) || time.Now().After(deadline) {
			return fmt.Errorf("helm %s failed: %s: %w", op, out, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("helm %s cancelled while awaiting plugin RBAC: %w", op, ctx.Err())
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

// ReleaseStatus is the part of `helm status -o json` a plugin needs to decide
// whether to (re)install a release.
type ReleaseStatus struct {
	// Status is Helm's release status: deployed, failed, pending-install,
	// pending-upgrade, pending-rollback, uninstalling, superseded or uninstalled.
	Status string
	// ChartVersion is the deployed chart's version, e.g. "0.24.0".
	ChartVersion string
}

// Pending reports whether a Helm operation holds the release. Another install
// fails with "another operation is in progress" until it finishes, or forever
// when the process running it was killed.
func (s *ReleaseStatus) Pending() bool {
	return strings.HasPrefix(s.Status, "pending-")
}

// Status returns the deployed state of a release in the client's namespace, or
// nil when the release does not exist. Like the installs, it retries while the
// plugin SA cannot yet read Helm's release Secrets (see the BANDAID note above):
// it typically runs first in Start, and failing there would restart the pod.
func (c *Client) Status(ctx context.Context, releaseName string) (*ReleaseStatus, error) {
	var stdout bytes.Buffer
	notFound := false
	err := retryOnRBACForbidden(ctx, "status", func() (string, error) {
		stdout.Reset()
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "helm", "status", releaseName, "--namespace", c.namespace, "--output", "json") //nolint:gosec // args are constructed internally
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		out := strings.TrimSpace(stderr.String())
		if err != nil && isReleaseNotFound(out) {
			notFound = true
			return out, nil
		}
		return out, err //nolint:wrapcheck // retryOnRBACForbidden wraps it with helm's output
	})
	if err != nil {
		return nil, err
	}
	if notFound {
		return nil, nil
	}
	return parseReleaseStatus(stdout.Bytes())
}

func isReleaseNotFound(output string) bool {
	return strings.Contains(output, "release: not found")
}

func parseReleaseStatus(data []byte) (*ReleaseStatus, error) {
	var release struct {
		Info struct {
			Status string `json:"status"`
		} `json:"info"`
		Chart struct {
			Metadata struct {
				Version string `json:"version"`
			} `json:"metadata"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return nil, fmt.Errorf("parse helm status: %w", err)
	}
	return &ReleaseStatus{Status: release.Info.Status, ChartVersion: release.Chart.Metadata.Version}, nil
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
