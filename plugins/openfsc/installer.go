package main

import (
	"bufio"
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

// OpenFSC v2.3.0 reference implementation (https://gitlab.com/rinis-oss/fsc/open-fsc).
// The v2 charts are NOT published to a Helm registry; they live in-repo under
// helm/charts/open-fsc-* and are assembled into the `shared` directory umbrella
// at helm/deploy/shared. We fetch them at install time (clone + dependency
// build) and install the umbrella, which stands up a Manager + Controller whose
// Manager functions as the group's Directory.
const (
	openFSCRepo    = "https://gitlab.com/rinis-oss/fsc/open-fsc.git"
	openFSCTag     = "v2.3.0"
	umbrellaSubdir = "helm/deploy/shared"

	releaseName = "shared"

	certManagerRepo  = "https://charts.jetstack.io"
	certManagerChart = "cert-manager"
	cnpgRepo         = "https://cloudnative-pg.github.io/charts"
	cnpgChart        = "cloudnative-pg"
)

// installer stands up a self-contained OpenFSC directory peer: it installs the
// prerequisite operators (cert-manager, CloudNativePG) and a `basic-csi`
// StorageClass, then clones and installs the upstream `shared` umbrella with the
// demo CA injected.
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
// with --wait (the umbrella's Certificates and CNPG Cluster need their CRDs and
// controllers present); the umbrella itself is applied without --wait so the
// operator can start promptly and the Peer reconciler tracks readiness.
func (i *installer) install(ctx context.Context, host pluginruntime.Host) error {
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing prerequisites (cert-manager, CloudNativePG)"})
	if err := i.ensurePrerequisites(ctx); err != nil {
		return fmt.Errorf("ensure prerequisites: %w", err)
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "fetching OpenFSC charts"})
	umbrella, err := i.fetchUmbrella(ctx)
	if err != nil {
		return fmt.Errorf("fetch OpenFSC charts: %w", err)
	}

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing OpenFSC Manager + Controller"})
	if err := i.installUmbrella(ctx, umbrella); err != nil {
		return fmt.Errorf("install OpenFSC umbrella: %w", err)
	}
	return nil
}

// ensurePrerequisites installs cert-manager and CloudNativePG (idempotent helm
// upgrade --install) and the `basic-csi` StorageClass the umbrella's CNPG
// Cluster hardcodes.
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
// local-path provisioner) that the umbrella's CNPG Cluster references.
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

// fetchUmbrella clones the OpenFSC repo at the pinned tag (cached across
// retries) and runs `helm dependency build` so the local file:// subcharts are
// packaged into the umbrella. It returns the umbrella chart directory.
func (i *installer) fetchUmbrella(ctx context.Context) (string, error) {
	src := filepath.Join(pluginWorkDir(), "open-fsc-"+openFSCTag)
	umbrella := filepath.Join(src, umbrellaSubdir)

	if _, err := os.Stat(filepath.Join(umbrella, "Chart.yaml")); err != nil {
		// Re-clone if the cache is absent or incomplete.
		_ = os.RemoveAll(src)
		if err := i.run(ctx, "git", "clone", "--quiet", "--depth", "1", "--branch", openFSCTag, openFSCRepo, src); err != nil {
			return "", fmt.Errorf("git clone: %w", err)
		}
	}

	if err := i.runHelmIn(ctx, umbrella, "dependency", "build"); err != nil {
		return "", fmt.Errorf("helm dependency build: %w", err)
	}
	return umbrella, nil
}

// installUmbrella installs the `shared` umbrella with the chart defaults, the
// embedded Fundament override, and the upstream demo CA keypair (extracted from
// the umbrella's values-demo.yaml and passed via --set-file so it is never on
// the command line).
func (i *installer) installUmbrella(ctx context.Context, umbrella string) error {
	override, err := i.writeOverride()
	if err != nil {
		return err
	}
	caCrt, caKey, err := i.extractDemoCA(umbrella)
	if err != nil {
		return fmt.Errorf("extract demo CA: %w", err)
	}

	return i.runHelm(ctx,
		"upgrade", "--install", releaseName, umbrella,
		"--namespace", i.cfg.Namespace, "--create-namespace",
		"-f", filepath.Join(umbrella, "values.yaml"),
		"-f", override,
		"--set", "global.groupID="+i.cfg.GroupID,
		"--set-file", "global.certificates.group.caCertificatePEM="+caCrt,
		"--set-file", "ca.issuer.certificatePEM="+caCrt,
		"--set-file", "ca.issuer.keyPEM="+caKey,
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

// extractDemoCA pulls the first CERTIFICATE and EC PRIVATE KEY PEM blocks out of
// the umbrella's values-demo.yaml (where they are indented under YAML keys),
// dedents them, and writes them to temp files. The demo CA is public (shipped in
// the upstream chart) and is only used for this self-contained dev directory.
func (i *installer) extractDemoCA(umbrella string) (crtPath, keyPath string, err error) {
	data, err := os.ReadFile(filepath.Join(umbrella, "values-demo.yaml"))
	if err != nil {
		return "", "", fmt.Errorf("read values-demo.yaml: %w", err)
	}
	crt, err := extractPEMBlock(string(data), "-----BEGIN CERTIFICATE-----", "-----END CERTIFICATE-----")
	if err != nil {
		return "", "", fmt.Errorf("certificate: %w", err)
	}
	key, err := extractPEMBlock(string(data), "-----BEGIN EC PRIVATE KEY-----", "-----END EC PRIVATE KEY-----")
	if err != nil {
		return "", "", fmt.Errorf("private key: %w", err)
	}

	crtPath = filepath.Join(pluginWorkDir(), "ca.crt")
	keyPath = filepath.Join(pluginWorkDir(), "ca.key")
	if err := os.WriteFile(crtPath, []byte(crt), 0o600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		return "", "", err
	}
	return crtPath, keyPath, nil
}

// extractPEMBlock returns the dedented PEM block bounded by begin/end markers.
func extractPEMBlock(content, begin, end string) (string, error) {
	var b strings.Builder
	in := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == begin {
			in = true
		}
		if in {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if line == end && in {
			return b.String(), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("PEM block %q not found", begin)
}

// uninstall removes the umbrella release. Prerequisite operators (cert-manager,
// CloudNativePG) are left in place; they may be shared with other plugins.
func (i *installer) uninstall(ctx context.Context) error {
	return i.runHelm(ctx, "uninstall", releaseName, "--namespace", i.cfg.Namespace)
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

func (i *installer) runHelmIn(ctx context.Context, dir string, args ...string) error {
	cmd := i.helm(ctx, args...)
	cmd.Dir = dir
	return i.runCmd(cmd)
}

func (i *installer) run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // args are constructed internally
	cmd.Env = helmEnv()
	return i.runCmd(cmd)
}

func (i *installer) runCmd(cmd *exec.Cmd) error {
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %w", cmd.Args[0], strings.TrimSpace(string(output)), err)
	}
	return nil
}
