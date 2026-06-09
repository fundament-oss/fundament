package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// OpenFSC is installed from the digilab umbrella chart
// (gitlab.com/digilab.overheid.nl/platform/helm-charts/open-fsc, version 1.43.0),
// vendored as charts/open-fsc-1.43.0.tgz and baked into the plugin image at
// /charts. The umbrella stands up a Manager + Controller (+ auditlog + txlog-api)
// whose Manager functions as the group's Directory.
//
// The umbrella ships only the internal mTLS CA, so the group (federation) CA, the
// Manager's group certificate and a Postgres cluster — all of which a
// self-contained directory peer needs — are provided by the openfsc-directory
// helper chart, installed first as release "shared-directory". See
// values-fundament.yaml for how the umbrella is wired to those resources.
const (
	// Vendored charts (see Dockerfile COPY plugins/openfsc/charts /charts).
	umbrellaChart  = "/charts/open-fsc-1.43.0.tgz"
	directoryChart = "/charts/openfsc-directory"

	releaseName      = "shared"
	directoryRelease = "shared-directory"

	certManagerRepo  = "https://charts.jetstack.io"
	certManagerChart = "cert-manager"
	cnpgRepo         = "https://cloudnative-pg.github.io/charts"
	cnpgChart        = "cloudnative-pg"
)

// installer stands up a self-contained OpenFSC directory peer: it installs the
// prerequisite operators (cert-manager, CloudNativePG) and a `basic-csi`
// StorageClass, then the openfsc-directory helper chart (group CA + Manager group
// cert + Postgres) and the OpenFSC umbrella.
type installer struct {
	cfg  pluginConfig
	kube client.Client
	log  *slog.Logger
}

func newInstaller(cfg pluginConfig, kube client.Client, log *slog.Logger) *installer {
	return &installer{cfg: cfg, kube: kube, log: log}
}

// isInstalled reports whether the `shared` umbrella release already exists.
func (i *installer) isInstalled(ctx context.Context) (bool, error) {
	cmd := i.helm(ctx, "status", releaseName, "--namespace", i.cfg.Namespace)
	if err := cmd.Run(); err != nil {
		return false, nil //nolint:nilerr // non-zero exit means release not found, not an error
	}
	return true, nil
}

// install runs the full standup. cert-manager and CloudNativePG are installed
// with --wait (the directory chart's Certificates and CNPG Cluster need their
// CRDs and controllers present); the helper chart and umbrella are applied without
// --wait so the operator can start promptly and the Peer reconciler tracks
// readiness.
func (i *installer) install(ctx context.Context, host pluginruntime.Host) error {
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing prerequisites (cert-manager, CloudNativePG)"})
	if err := i.ensurePrerequisites(ctx); err != nil {
		return fmt.Errorf("ensure prerequisites: %w", err)
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "provisioning directory CA and database"})
	if err := i.installDirectory(ctx); err != nil {
		return fmt.Errorf("install directory prerequisites: %w", err)
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing OpenFSC Manager + Controller"})
	if err := i.installUmbrella(ctx); err != nil {
		return fmt.Errorf("install OpenFSC umbrella: %w", err)
	}
	return nil
}

// ensurePrerequisites installs cert-manager and CloudNativePG (idempotent helm
// upgrade --install) and the `basic-csi` StorageClass the directory chart's CNPG
// Cluster references.
func (i *installer) ensurePrerequisites(ctx context.Context) error {
	if err := i.runHelm(ctx,
		"upgrade", "--install", "cert-manager", certManagerChart,
		"--repo", certManagerRepo, "--namespace", "cert-manager", "--create-namespace",
		"--set", "crds.enabled=true", "--wait", "--timeout", "5m",
	); err != nil {
		return fmt.Errorf("install cert-manager: %w", err)
	}

	if err := i.runHelm(ctx,
		"upgrade", "--install", "cnpg", cnpgChart,
		"--repo", cnpgRepo, "--namespace", "cnpg-system", "--create-namespace",
		"--wait", "--timeout", "5m",
	); err != nil {
		return fmt.Errorf("install cloudnative-pg: %w", err)
	}

	return i.ensureStorageClass(ctx)
}

// ensureStorageClass creates the `basic-csi` StorageClass (an alias of the k3s
// local-path provisioner) that the directory chart's CNPG Cluster references.
func (i *installer) ensureStorageClass(ctx context.Context) error {
	volumeBindingMode := storagev1.VolumeBindingWaitForFirstConsumer
	reclaim := corev1.PersistentVolumeReclaimDelete
	sc := &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: "basic-csi"},
		Provisioner:       "rancher.io/local-path",
		VolumeBindingMode: &volumeBindingMode,
		ReclaimPolicy:     &reclaim,
	}
	if err := i.kube.Create(ctx, sc); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create basic-csi StorageClass: %w", err)
	}
	return nil
}

// installDirectory installs the openfsc-directory helper chart: the self-signed
// group CA + group Issuer "shared", the Manager's group certificate, and the
// CloudNativePG cluster the umbrella's components share.
func (i *installer) installDirectory(ctx context.Context) error {
	return i.runHelm(ctx,
		"upgrade", "--install", directoryRelease, directoryChart,
		"--namespace", i.cfg.Namespace, "--create-namespace",
		"--set", "directoryPeerID="+i.cfg.DirectoryPeerID,
	)
}

// installUmbrella installs the vendored OpenFSC umbrella as release "shared" with
// fullnameOverride=shared (so its internal-TLS certificate CommonNames/SANs match
// the subchart service names) and the embedded Fundament override.
func (i *installer) installUmbrella(ctx context.Context) error {
	override, err := i.writeOverride()
	if err != nil {
		return err
	}
	return i.runHelm(ctx,
		"upgrade", "--install", releaseName, umbrellaChart,
		"--namespace", i.cfg.Namespace, "--create-namespace",
		"-f", override,
		"--set", "fullnameOverride=shared",
		"--set", "global.groupID="+i.cfg.GroupID,
		"--set", "open-fsc-manager.config.groupID="+i.cfg.GroupID,
		"--set", "open-fsc-manager.config.directoryPeerID="+i.cfg.DirectoryPeerID,
	)
}

// writeOverride writes the embedded values-fundament.yaml to a temp file for -f.
func (i *installer) writeOverride() (string, error) {
	path := filepath.Join(pluginWorkDir(), "values-fundament.yaml")
	if err := os.WriteFile(path, valuesFundament, 0o600); err != nil {
		return "", fmt.Errorf("write override values: %w", err)
	}
	return path, nil
}

// uninstall removes the umbrella and directory helper releases. Prerequisite
// operators (cert-manager, CloudNativePG) are left in place; they may be shared
// with other plugins.
func (i *installer) uninstall(ctx context.Context) error {
	if err := i.runHelm(ctx, "uninstall", releaseName, "--namespace", i.cfg.Namespace, "--ignore-not-found"); err != nil {
		return err
	}
	return i.runHelm(ctx, "uninstall", directoryRelease, "--namespace", i.cfg.Namespace, "--ignore-not-found")
}

// helm builds a helm command with HELM_* env pointed at the writable work dir.
func (i *installer) helm(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "helm", args...) //nolint:gosec // args are constructed internally
	cmd.Env = helmEnv()
	return cmd
}

func (i *installer) runHelm(ctx context.Context, args ...string) error {
	return i.runCmd(i.helm(ctx, args...))
}

func (i *installer) runCmd(cmd *exec.Cmd) error {
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %w", cmd.Args[0], strings.TrimSpace(string(output)), err)
	}
	return nil
}
