// Package cnpg installs the CloudNativePG operator for the plugins that need it
// (cloudnativepg, openfsc). They share one Helm release, so they share the
// chart pin and the install rules here.
//
// A shared constant keeps plugins in step only when their images come from the
// same commit; installed plugins can run different versions. Install therefore
// never downgrades the release: an older plugin leaves a newer operator (and the
// CRDs its chart ships as templates) in place.
package cnpg

import (
	"context"
	"fmt"

	"golang.org/x/mod/semver"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	Release   = "cnpg"
	Namespace = "cnpg-system"
	Repo      = "https://cloudnative-pg.github.io/charts"
	Chart     = "cloudnative-pg"
	Version   = "0.24.0" // ships CloudNativePG operator v1.26.0
)

// CRDNames are the CRDs a plugin verifies after Install.
var CRDNames = []string{
	"clusters.postgresql.cnpg.io",
}

// Install installs or upgrades the shared operator release to Version. It is
// a no-op when the release is already deployed at Version or newer, so a
// plugin restart does not re-run `helm upgrade --wait`. Values stay at the
// chart defaults: two plugins writing different values would flip them on
// every install.
func Install(ctx context.Context) error {
	h := helm.NewClient(Namespace)
	current, err := h.Status(ctx, Release)
	if err != nil {
		return fmt.Errorf("read %s release: %w", Release, err)
	}
	install, err := needsInstall(current, Version)
	if err != nil || !install {
		return err
	}
	if err := h.InstallFromRepo(ctx, Release, Chart, Repo, Version, nil); err != nil {
		return fmt.Errorf("install cloudnative-pg: %w", err)
	}
	return nil
}

// needsInstall decides whether to run `helm upgrade --install` against the
// release in its current state (nil when absent) to reach version want.
func needsInstall(current *helm.ReleaseStatus, want string) (bool, error) {
	if current == nil {
		return true, nil
	}
	if current.Pending() {
		// Another plugin may be mid-install; retrying resolves that. A release
		// whose install was killed during --wait stays pending for good.
		return false, fmt.Errorf("release %s/%s is %s; if this persists, run `%s`",
			Namespace, Release, current.Status, pendingRecovery(current.Status))
	}
	if current.Status == "deployed" && semver.Compare("v"+current.ChartVersion, "v"+want) >= 0 {
		return false, nil
	}
	return true, nil
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
