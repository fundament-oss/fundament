// Package cnpg installs the CloudNativePG operator. The openfsc plugin installs
// the same Helm release with its own `helm upgrade --install`, since a cluster
// runs one operator for the CRDs, so this release can be held or upgraded by
// someone else.
//
// Install therefore never downgrades the release: a newer operator (and the
// CRDs its chart ships as templates) stays in place, whatever state the release
// is in.
package cnpg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	Release   = "cnpg"
	Namespace = "cnpg-system"
	Repo      = "https://cloudnative-pg.github.io/charts"
	Chart     = "cloudnative-pg"
	Version   = "0.24.0" // ships CloudNativePG operator v1.26.0
)

// pendingGrace is how long a pending or uninstalling release counts as an
// operation in progress. Past it, the operation that held the release was most
// likely killed. It covers helm's default --wait timeout (5m) with slack.
const pendingGrace = 10 * time.Minute

// CRDNames are the CRDs the plugin verifies after Install.
var CRDNames = []string{
	"clusters.postgresql.cnpg.io",
}

// ErrInProgress marks an Install that found another helm operation on the
// release still running: another plugin's install, or an admin's uninstall.
// Callers report it as installing, not degraded, and retry.
var ErrInProgress = errors.New("install in progress")

// Install installs or upgrades the operator release to Version. When
// the release is already deployed at Version or newer and the operator's
// Deployment and CRDs are all there, it does nothing, so a plugin restart
// does not re-run `helm upgrade --wait`. When one of them was deleted, it
// re-runs the install at the deployed version to restore them. Values stay at
// the chart defaults: two plugins writing different values would flip them on
// every install.
func Install(ctx context.Context, kube client.Client) error {
	h := helm.NewClient(Namespace)
	current, err := h.Status(ctx, Release)
	if err != nil {
		return fmt.Errorf("read %s release: %w", Release, err)
	}
	version, err := installVersion(current, time.Now())
	if err != nil {
		return err
	}
	if current != nil && current.Status == "deployed" && version == current.ChartVersion {
		present, err := OperatorPresent(ctx, kube)
		if err != nil {
			return err
		}
		if present {
			return nil
		}
	}
	if err := h.InstallFromRepo(ctx, Release, Chart, Repo, version, nil); err != nil {
		// openfsc can read the release absent too and install it first; this
		// install then finds openfsc's holding it.
		if concurrentOperation(err) {
			return fmt.Errorf("%w: another install of release %s/%s started first: %w", ErrInProgress, Namespace, Release, err)
		}
		return fmt.Errorf("helm install %s: %w", Chart, err)
	}
	return nil
}

// concurrentOperation reports whether helm refused an install because another
// operation holds the release: "another operation (install/upgrade/rollback)
// is in progress" once that operation recorded the release, or "cannot re-use
// a name that is still in use" when both started from an absent release.
func concurrentOperation(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "another operation") || strings.Contains(msg, "cannot re-use a name that is still in use")
}

// installVersion returns the chart version to install over the release in its
// current state (nil when absent): the newer of Version and the release's own,
// so a failed or uninstalled release left by a newer plugin is retried at its
// own version rather than downgraded. It refuses while another helm operation
// holds the release, including an uninstall: upgrading a release that is being
// deleted would collide with it.
func installVersion(current *helm.ReleaseStatus, now time.Time) (string, error) {
	if current == nil {
		return Version, nil
	}
	if current.Pending() || current.Status == "uninstalling" {
		return "", pendingError(current, now)
	}
	if semver.Compare("v"+current.ChartVersion, "v"+Version) > 0 {
		return current.ChartVersion, nil
	}
	return Version, nil
}

// pendingError explains a pending or uninstalling release. Within pendingGrace
// it is an operation still running, which a retry resolves; after it, the
// operation was most likely killed and the release stays in that state for
// good until an admin clears it.
func pendingError(current *helm.ReleaseStatus, now time.Time) error {
	// An uninstall keeps the release's deploy time and records its own start
	// as the deletion time.
	since := current.LastDeployed
	if current.Status == "uninstalling" {
		since = current.Deleted
	}
	if !since.IsZero() && now.Sub(since) < pendingGrace {
		return fmt.Errorf("%w: release %s/%s is %s since %s; waiting for it to finish",
			ErrInProgress, Namespace, Release, current.Status, since.UTC().Format(time.RFC3339))
	}
	return fmt.Errorf("release %s/%s has been %s for over %s; %s",
		Namespace, Release, current.Status, pendingGrace, pendingRecovery(current.Status))
}

// pendingRecovery says how to clear a pending release. A first install has no
// earlier revision to roll back to, so it is uninstalled instead. That is not
// safe by itself: --wait may have been killed after the operator and CRDs were
// created, and the chart ships the CRDs as templates, so the uninstall deletes
// them and every Cluster with them. An interrupted uninstall already started
// deleting them, so it is finished rather than undone.
func pendingRecovery(status string) string {
	switch status {
	case "uninstalling":
		return fmt.Sprintf("run `helm -n %s uninstall %s` to finish it", Namespace, Release)
	case "pending-install":
		return fmt.Sprintf("check that `kubectl get %s -A` lists no databases, then run `helm -n %s uninstall %s` to clear it",
			CRDNames[0], Namespace, Release)
	default:
		return fmt.Sprintf("run `helm -n %s rollback %s` to clear it", Namespace, Release)
	}
}

// OperatorPresent reports whether the release's operator Deployment and the
// CRDs still exist. Helm keeps saying deployed after either is deleted, and
// Install is what puts them back.
func OperatorPresent(ctx context.Context, kube client.Client) (bool, error) {
	var deployments appsv1.DeploymentList
	if err := kube.List(ctx, &deployments, client.InNamespace(Namespace), client.MatchingLabels{
		"app.kubernetes.io/name":     Chart,
		"app.kubernetes.io/instance": Release,
	}); err != nil {
		return false, fmt.Errorf("list operator deployments: %w", err)
	}
	if len(deployments.Items) == 0 {
		return false, nil
	}
	for _, name := range CRDNames {
		ok, err := crd.Exists(ctx, kube, name)
		if err != nil || !ok {
			return false, err //nolint:wrapcheck // crd.Exists names the CRD
		}
	}
	return true, nil
}
