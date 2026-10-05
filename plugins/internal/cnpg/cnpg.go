// Package cnpg installs the CloudNativePG operator for the plugins that need it
// (cloudnativepg, openfsc). They share one Helm release, so they share the
// chart pin and the install rules here.
//
// A shared constant keeps plugins in step only when their images come from the
// same commit; installed plugins can run different versions. Install therefore
// never downgrades the release: an older plugin leaves a newer operator (and the
// CRDs its chart ships as templates) in place, whatever state the release is in.
package cnpg

import (
	"context"
	"errors"
	"fmt"
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

// pendingGrace is how long a pending release counts as another plugin's
// install in progress. Past it, the operation that held the release was most
// likely killed. It covers helm's default --wait timeout (5m) with slack.
const pendingGrace = 10 * time.Minute

// CRDNames are the CRDs a plugin verifies after Install.
var CRDNames = []string{
	"clusters.postgresql.cnpg.io",
}

// ErrInProgress marks an Install that found another plugin's helm operation on
// the release still running. Callers report it as installing, not degraded,
// and retry.
var ErrInProgress = errors.New("install in progress")

// Install installs or upgrades the shared operator release to Version. When
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
		present, err := operatorPresent(ctx, kube)
		if err != nil {
			return err
		}
		if present {
			return nil
		}
	}
	if err := h.InstallFromRepo(ctx, Release, Chart, Repo, version, nil); err != nil {
		return fmt.Errorf("helm install %s: %w", Chart, err)
	}
	return nil
}

// installVersion returns the chart version to install over the release in its
// current state (nil when absent): the newer of Version and the release's own, so a failed or uninstalled release left by a newer plugin is
// retried at its own version rather than downgraded. It refuses while another
// helm operation holds the release.
func installVersion(current *helm.ReleaseStatus, now time.Time) (string, error) {
	if current == nil {
		return Version, nil
	}
	if current.Pending() {
		return "", pendingError(current, now)
	}
	if semver.Compare("v"+current.ChartVersion, "v"+Version) > 0 {
		return current.ChartVersion, nil
	}
	return Version, nil
}

// pendingError explains a pending release. Within pendingGrace it is another
// plugin's install still running, which a retry resolves; after it, the
// operation was most likely killed during --wait and the release stays
// pending for good until an admin clears it.
func pendingError(current *helm.ReleaseStatus, now time.Time) error {
	if !current.LastDeployed.IsZero() && now.Sub(current.LastDeployed) < pendingGrace {
		return fmt.Errorf("%w: release %s/%s is %s since %s; waiting for it to finish",
			ErrInProgress, Namespace, Release, current.Status, current.LastDeployed.UTC().Format(time.RFC3339))
	}
	return fmt.Errorf("release %s/%s has been %s for over %s; run `%s` to clear it",
		Namespace, Release, current.Status, pendingGrace, pendingRecovery(current.Status))
}

// pendingRecovery is the command that clears a pending release. A first install
// has no earlier revision to roll back to, so it is uninstalled instead; that
// is safe because it never deployed, so no databases depend on it yet.
func pendingRecovery(status string) string {
	if status == "pending-install" {
		return fmt.Sprintf("helm -n %s uninstall %s", Namespace, Release)
	}
	return fmt.Sprintf("helm -n %s rollback %s", Namespace, Release)
}

// operatorPresent reports whether the release's operator Deployment and the
// CRDs still exist. Helm keeps saying deployed after either is deleted, and
// `helm upgrade --install` is what puts them back.
func operatorPresent(ctx context.Context, kube client.Client) (bool, error) {
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
