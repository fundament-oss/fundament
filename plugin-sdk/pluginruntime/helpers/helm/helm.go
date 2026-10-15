// Package helm provides optional helpers for managing Helm releases from plugins.
// It shells out to the helm CLI, which must be available in the container's PATH.
package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// IsInstalled checks whether a Helm release exists in the client's namespace,
// in any state. Only a missing release reports false; any other helm failure
// (cluster unreachable, RBAC still forbidden after the retries) is an error.
// Unlike Status, it needs no chart version, so it runs only `helm status`.
func (c *Client) IsInstalled(ctx context.Context, releaseName string) (bool, error) {
	status, err := c.readRelease(ctx, "status", releaseName)
	if err != nil {
		return false, err
	}
	return status != nil, nil
}

// ReleaseStatus is what a plugin needs to decide whether to (re)install a
// release: `helm status -o json` for the state and its timestamps, and
// `helm get metadata -o json` for the chart version, since helm status strips
// the chart from its output.
type ReleaseStatus struct {
	// Status is Helm's release status: deployed, failed, pending-install,
	// pending-upgrade, pending-rollback, uninstalling, superseded or uninstalled.
	Status string
	// Revision is the latest revision's number, which Status and ChartVersion
	// both describe.
	Revision int
	// ChartVersion is the latest revision's chart version, e.g. "0.24.0".
	ChartVersion string
	// LastDeployed is when the latest operation on the release started; for a
	// pending release, when that operation began. Zero when helm omits it.
	LastDeployed time.Time
	// Deleted is when an uninstall started; set for an uninstalling or
	// uninstalled release, zero otherwise.
	Deleted time.Time
}

// Pending reports whether a Helm operation holds the release. Another install
// fails with "another operation is in progress" until it finishes, or forever
// when the process running it was killed.
func (s *ReleaseStatus) Pending() bool {
	return strings.HasPrefix(s.Status, "pending-")
}

// statusReadAttempts bounds how often Status reads the release again when an
// operation recorded a new revision between its two helm calls.
const statusReadAttempts = 3

// Status returns the state of a release's latest revision in the client's
// namespace, or nil when the release does not exist. Like the installs, it
// retries while the plugin SA cannot yet read Helm's release Secrets (see the
// BANDAID note above): it typically runs first in Start, and failing there
// would restart the pod.
//
// The state and the chart version come from two helm calls. An operation that
// records a new revision between them would pair the old revision's state
// with the new one's version, so Status reads both again until they name the
// same revision.
func (c *Client) Status(ctx context.Context, releaseName string) (*ReleaseStatus, error) {
	for range statusReadAttempts {
		statusJSON, err := c.readRelease(ctx, "status", releaseName)
		if err != nil || statusJSON == nil {
			return nil, err
		}
		status, err := parseReleaseStatus(statusJSON)
		if err != nil {
			return nil, err
		}
		metadataJSON, err := c.readRelease(ctx, "get metadata", releaseName)
		if err != nil {
			return nil, err
		}
		if metadataJSON == nil {
			// Uninstalled between the two calls.
			return nil, nil
		}
		version, revision, err := parseChartVersion(metadataJSON)
		if err != nil {
			return nil, err
		}
		if status.Revision != revision {
			continue
		}
		status.ChartVersion = version
		return status, nil
	}
	return nil, fmt.Errorf("release %s kept changing while it was read", releaseName)
}

// readRelease runs a read-only helm command (op is "status" or "get metadata")
// with JSON output and returns its stdout, or nil when the release does not
// exist.
func (c *Client) readRelease(ctx context.Context, op, releaseName string) ([]byte, error) {
	var stdout bytes.Buffer
	notFound := false
	args := append(strings.Fields(op), releaseName, "--namespace", c.namespace, "--output", "json")
	err := retryOnRBACForbidden(ctx, op, func() (string, error) {
		stdout.Reset()
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "helm", args...) //nolint:gosec // args are constructed internally
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
	return stdout.Bytes(), nil
}

func isReleaseNotFound(output string) bool {
	return strings.Contains(output, "release: not found")
}

// parseReleaseStatus reads `helm status -o json`. Its ChartVersion stays empty:
// helm status has no chart in its output.
func parseReleaseStatus(data []byte) (*ReleaseStatus, error) {
	var release struct {
		// Version is the revision number, not the chart's version.
		Version int `json:"version"`
		Info    struct {
			Status       string `json:"status"`
			LastDeployed string `json:"last_deployed"`
			Deleted      string `json:"deleted"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return nil, fmt.Errorf("parse helm status: %w", err)
	}
	// A missing or unparseable timestamp stays zero, which a caller reads as
	// "long ago".
	lastDeployed, _ := time.Parse(time.RFC3339Nano, release.Info.LastDeployed)
	deleted, _ := time.Parse(time.RFC3339Nano, release.Info.Deleted)
	return &ReleaseStatus{
		Status:       release.Info.Status,
		Revision:     release.Version,
		LastDeployed: lastDeployed,
		Deleted:      deleted,
	}, nil
}

// parseChartVersion reads the chart version and the revision it belongs to
// from `helm get metadata -o json`.
func parseChartVersion(data []byte) (version string, revision int, err error) {
	var metadata struct {
		Version  string `json:"version"`
		Revision int    `json:"revision"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", 0, fmt.Errorf("parse helm get metadata: %w", err)
	}
	if metadata.Version == "" {
		return "", 0, errors.New("helm get metadata: no chart version")
	}
	return metadata.Version, metadata.Revision, nil
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
