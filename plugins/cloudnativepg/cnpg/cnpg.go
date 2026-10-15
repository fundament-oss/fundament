// Package cnpg installs the CloudNativePG operator under a Helm release of its
// own and refuses to share the operator with anyone else. A cluster runs one
// operator for the CRDs, and every operator version change restarts every
// database on the cluster, so two writers that do not know each other's
// versions or upgrade timing would restart them at will. ForeignOwner tells
// whether the operator on the cluster is someone else's.
//
// Install never downgrades the release: a plugin rollback leaves a newer
// operator (and the CRDs its chart ships as templates) in place, whatever state
// the release is in.
package cnpg

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	// Release is not the chart's usual `cnpg`, which openfsc installs: helm's
	// ownership check then refuses either install over the other.
	Release   = "fundament-cloudnativepg"
	Namespace = "cnpg-system"
	Repo      = "https://cloudnative-pg.github.io/charts"
	Chart     = "cloudnative-pg"
	Version   = "0.24.0" // ships CloudNativePG operator v1.26.0
)

// pendingGrace is how long a pending or uninstalling release counts as an
// operation in progress. Past it, the operation that held the release was most
// likely killed. It covers helm's default --wait timeout (5m) with slack.
const pendingGrace = 10 * time.Minute

// CRDNames are the CRDs the plugin verifies after Install. The first one
// carries the Helm annotations ForeignOwner reads.
var CRDNames = []string{
	"clusters.postgresql.cnpg.io",
	"clusterimagecatalogs.postgresql.cnpg.io",
}

// ErrInProgress marks an Install that found another helm operation on the
// release still running: an earlier plugin pod's install during a rolling
// update, or an admin's uninstall. Callers report it as installing, not
// degraded, and retry.
var ErrInProgress = errors.New("install in progress")

// ErrForeignOperator marks an Install that found a CloudNativePG operator this
// plugin did not install. Callers report it as failed and leave the operator
// alone.
var ErrForeignOperator = errors.New("the CloudNativePG operator on this cluster was installed by someone else, and the plugin does not share it")

// Install installs or upgrades the operator release to Version. When the
// release is already deployed at Version or newer and the operator's
// Deployment and CRDs are all there, it does nothing, so a plugin restart does
// not re-run `helm upgrade --wait`. When one of them was deleted, it re-runs
// the install at the deployed version to restore them. It refuses an operator
// someone else installed.
func Install(ctx context.Context, kube client.Client) error {
	owner, err := ForeignOwner(ctx, kube)
	if err != nil {
		return err
	}
	if owner != "" {
		return fmt.Errorf("%w: %s", ErrForeignOperator, owner)
	}

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
		if concurrentOperation(err) {
			return fmt.Errorf("%w: another install of release %s/%s started first: %w", ErrInProgress, Namespace, Release, err)
		}
		return fmt.Errorf("helm install %s: %w", Chart, err)
	}
	return nil
}

// Uninstall removes the operator release when it exists. The chart marks its
// CRDs helm.sh/resource-policy: keep, so they stay, and with them their Helm
// annotations: a later Install adopts them again.
func Uninstall(ctx context.Context) error {
	h := helm.NewClient(Namespace)
	current, err := h.Status(ctx, Release)
	if err != nil {
		return fmt.Errorf("read %s release: %w", Release, err)
	}
	if current == nil {
		return nil
	}
	if err := h.Uninstall(ctx, Release); err != nil {
		return fmt.Errorf("helm uninstall %s: %w", Release, err)
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
// so a plugin rollback, or a failed or uninstalled release left by a newer
// plugin, keeps the newer version rather than downgrading it. It refuses while
// another helm operation holds the release, including an uninstall: upgrading
// a release that is being deleted would collide with it.
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
// earlier revision to roll back to, so it is uninstalled instead, which is
// safe: the chart's CRDs carry helm.sh/resource-policy: keep, so the databases
// stay, and the plugin installs the operator again. An interrupted uninstall
// is finished rather than undone.
func pendingRecovery(status string) string {
	switch status {
	case "uninstalling", "pending-install":
		return fmt.Sprintf("run `helm -n %s uninstall %s` to clear it; the databases stay and the plugin reinstalls the operator", Namespace, Release)
	default:
		return fmt.Sprintf("run `helm -n %s rollback %s` to clear it", Namespace, Release)
	}
}

// ForeignOwner returns who else runs the CloudNativePG operator on the
// cluster, and how to hand it over to this plugin, or "" when nobody else
// does: there is no CRD yet, or the CRD is this plugin's release's and no other
// operator Deployment runs. CRDs kept from this plugin's own uninstall still
// name its release, so a reinstall counts as its own. CRDs kept from someone
// else's uninstall still name theirs, so the plugin goes ahead only once they
// are pointed at its release: it does not take them over by itself.
//
// It cannot prevent a takeover: an org admin can `kubectl apply` the CNPG
// manifests or `helm install --take-ownership`, which skip helm's ownership
// check. It makes one visible.
func ForeignOwner(ctx context.Context, kube client.Client) (string, error) {
	var clusters apiextensionsv1.CustomResourceDefinition
	err := kube.Get(ctx, client.ObjectKey{Name: CRDNames[0]}, &clusters)
	switch {
	case apierrors.IsNotFound(err):
	case err != nil:
		return "", fmt.Errorf("get CRD %s: %w", CRDNames[0], err)
	default:
		release := clusters.Annotations["meta.helm.sh/release-name"]
		namespace := clusters.Annotations["meta.helm.sh/release-namespace"]
		if release == "" {
			return fmt.Sprintf("CRD %s was not installed with Helm. To hand the operator to this plugin, remove that install but keep its CRDs (deleting them deletes every database), %s",
				CRDNames[0], adoptCRDs), nil
		}
		if release != Release || namespace != Namespace {
			return fmt.Sprintf("CRD %s belongs to Helm release %s/%s. To hand the operator to this plugin, run `helm -n %s uninstall %s` (its CRDs and databases stay), %s",
				CRDNames[0], namespace, release, namespace, release, adoptCRDs), nil
		}
	}

	var deployments appsv1.DeploymentList
	if err := kube.List(ctx, &deployments); err != nil {
		return "", fmt.Errorf("list deployments: %w", err)
	}
	for i := range deployments.Items {
		d := &deployments.Items[i]
		if d.Namespace == Namespace && d.Labels["app.kubernetes.io/instance"] == Release {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if isOperatorImage(c.Image) {
				return fmt.Sprintf("Deployment %s/%s runs the CloudNativePG operator. To hand the operator to this plugin, delete that Deployment", d.Namespace, d.Name), nil
			}
		}
	}
	return "", nil
}

// adoptCRDs ends ForeignOwner's hand-over for CRDs someone else installed:
// Helm adopts an existing object only when it carries the release's
// annotations and Helm's managed-by label.
var adoptCRDs = fmt.Sprintf("then label every postgresql.cnpg.io CRD app.kubernetes.io/managed-by=Helm and annotate it meta.helm.sh/release-name=%s meta.helm.sh/release-namespace=%s",
	Release, Namespace)

// isOperatorImage reports whether an image is the CloudNativePG operator's,
// from any registry: its repository ends in cloudnative-pg. PostgreSQL images
// end in postgresql.
func isOperatorImage(image string) bool {
	repository, _, _ := strings.Cut(image, "@")
	// A colon after the last slash starts the tag; one before it is a
	// registry port.
	if colon := strings.LastIndex(repository, ":"); colon > strings.LastIndex(repository, "/") {
		repository = repository[:colon]
	}
	return path.Base(repository) == "cloudnative-pg"
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
