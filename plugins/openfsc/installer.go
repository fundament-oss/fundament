package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	// operatorChart is baked into the plugin image by the Dockerfile.
	operatorChart     = "/operator-chart"
	operatorRelease   = "openfsc-operator"
	operatorNamespace = "openfsc-system"

	certManagerRepo    = "https://charts.jetstack.io"
	certManagerChart   = "cert-manager"
	certManagerVersion = "v1.17.2"
	cnpgRepo           = "https://cloudnative-pg.github.io/charts"
	cnpgChart          = "cloudnative-pg"
	cnpgVersion        = "0.24.0" // ships CloudNativePG operator v1.25.1
)

var crdNames = []string{
	"fscinstallations.openfsc.fundament.io",
}

type installer struct {
	cfg  *pluginConfig
	kube client.Client
	log  *slog.Logger

	operatorHelm *helm.Client
}

func newInstaller(cfg *pluginConfig, kube client.Client, log *slog.Logger) *installer {
	return &installer{cfg: cfg, kube: kube, log: log, operatorHelm: helm.NewClient(operatorNamespace)}
}

func (i *installer) isInstalled(ctx context.Context) (bool, error) {
	installed, err := i.operatorHelm.IsInstalled(ctx, operatorRelease)
	if err != nil {
		return false, fmt.Errorf("check openfsc-operator release: %w", err)
	}
	return installed, nil
}

func (i *installer) install(ctx context.Context, host pluginruntime.Host) error {
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing prerequisites (cert-manager, CloudNativePG)"})
	if err := i.ensurePrerequisites(ctx); err != nil {
		return fmt.Errorf("ensure prerequisites: %w", err)
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing openfsc-operator"})
	if err := i.installOperator(ctx); err != nil {
		return fmt.Errorf("install openfsc-operator: %w", err)
	}
	if err := waitEstablished(ctx, i.kube, crdNames); err != nil {
		return fmt.Errorf("wait for CRDs to be established: %w", err)
	}
	return nil
}

// ensurePrerequisites installs cert-manager and CloudNativePG. The operator
// preflights these but never installs them itself, reporting
// PrerequisitesMet=False on installations while they are missing.
func (i *installer) ensurePrerequisites(ctx context.Context) error {
	if err := helm.NewClient("cert-manager").InstallFromRepo(ctx, "cert-manager", certManagerChart, certManagerRepo, certManagerVersion, map[string]string{
		"crds.enabled": "true",
	}); err != nil {
		return fmt.Errorf("install cert-manager: %w", err)
	}

	if err := helm.NewClient("cnpg-system").InstallFromRepo(ctx, "cnpg", cnpgChart, cnpgRepo, cnpgVersion, nil); err != nil {
		return fmt.Errorf("install cloudnative-pg: %w", err)
	}
	return nil
}

func (i *installer) installOperator(ctx context.Context) error {
	repository, tag := splitImageRef(i.cfg.OperatorImage)
	values := map[string]string{
		"image.repository": repository,
	}
	if tag != "" {
		values["image.tag"] = tag
	}
	if err := i.operatorHelm.Install(ctx, operatorRelease, operatorChart, values); err != nil {
		return fmt.Errorf("install operator chart: %w", err)
	}
	return nil
}

// splitImageRef splits an image reference on the last colon after the last
// slash, so registry ports (localhost:5112/...) stay in the repository.
func splitImageRef(ref string) (repository, tag string) {
	idx := strings.LastIndex(ref, ":")
	if idx == -1 || strings.Contains(ref[idx:], "/") {
		return ref, ""
	}
	return ref[:idx], ref[idx+1:]
}

func waitEstablished(ctx context.Context, c client.Client, names []string) error {
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, name := range names {
			var crd apiextensionsv1.CustomResourceDefinition
			if err := c.Get(ctx, types.NamespacedName{Name: name}, &crd); err != nil {
				if apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
					return false, nil // not yet visible; keep polling
				}
				return false, err
			}
			established := false
			for _, cond := range crd.Status.Conditions {
				if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
					established = true
				}
			}
			if !established {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait for established CRDs: %w", err)
	}
	return nil
}

// uninstall removes the openfsc-operator release, but refuses while
// FSCInstallations exist: the resources belong to teams, and removing the
// operator first would strand their finalizers. cert-manager and CloudNativePG
// are left in place; they may be shared with other plugins.
func (i *installer) uninstall(ctx context.Context) error {
	var list openfscv1.FSCInstallationList
	err := i.kube.List(ctx, &list)
	if err != nil && !apierrors.IsNotFound(err) && !apimeta.IsNoMatchError(err) {
		return fmt.Errorf("list FSCInstallations: %w", err)
	}
	if len(list.Items) > 0 {
		names := make([]string, 0, len(list.Items))
		for idx := range list.Items {
			names = append(names, list.Items[idx].Namespace+"/"+list.Items[idx].Name)
		}
		return fmt.Errorf("%d FSCInstallation(s) still exist (%s); delete them first so the operator can tear them down, then uninstall the plugin", len(names), strings.Join(names, ", "))
	}

	if err := i.operatorHelm.Uninstall(ctx, operatorRelease); err != nil {
		return fmt.Errorf("uninstall openfsc-operator: %w", err)
	}
	return nil
}
