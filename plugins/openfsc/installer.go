package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

// The plugin is a thin installer around the standalone openfsc-operator: it
// installs the prerequisite operators the openfsc-operator preflights but never
// installs itself (cert-manager, CloudNativePG), the `basic-csi` StorageClass
// (a sandbox concern), and the operator's own chart (vendored into the plugin
// image at /operator-chart, CRDs included). It then declares the directory as
// a Directory resource and seeds a default Inway/Outway pair; the operator
// reconciles everything from there.
const (
	// operatorChart is the openfsc-operator chart baked into the plugin image
	// (see Dockerfile COPY openfsc-operator/chart /operator-chart).
	operatorChart     = "/operator-chart"
	operatorRelease   = "openfsc-operator"
	operatorNamespace = "openfsc-system"

	certManagerRepo  = "https://charts.jetstack.io"
	certManagerChart = "cert-manager"
	cnpgRepo         = "https://cloudnative-pg.github.io/charts"
	cnpgChart        = "cloudnative-pg"
)

// directoryName is the name of the Directory resource the plugin manages.
const directoryName = "default"

// Names of the default Inway/Outway the plugin ships with (seeded on install,
// then provisioned by the operator's gateway reconcilers). They must be
// distinct: the helm release name is the CR's metadata.name, so a shared name
// would collide.
const (
	defaultInwayName  = "default-inway"
	defaultOutwayName = "default-outway"
)

// crdNames are the openfsc.fundament.io CRDs the operator chart ships and the
// plugin waits on before creating resources.
var crdNames = []string{
	"directories.openfsc.fundament.io",
	"peers.openfsc.fundament.io",
	"inways.openfsc.fundament.io",
	"outways.openfsc.fundament.io",
}

// installer stands up the openfsc-operator and its prerequisites, then declares
// the directory and default gateways as custom resources.
type installer struct {
	cfg  *pluginConfig
	kube client.Client
	log  *slog.Logger

	operatorHelm *helm.Client
}

func newInstaller(cfg *pluginConfig, kube client.Client, log *slog.Logger) *installer {
	return &installer{cfg: cfg, kube: kube, log: log, operatorHelm: helm.NewClient(operatorNamespace)}
}

// isInstalled reports whether the openfsc-operator release already exists.
func (i *installer) isInstalled(ctx context.Context) (bool, error) {
	installed, err := i.operatorHelm.IsInstalled(ctx, operatorRelease)
	if err != nil {
		return false, fmt.Errorf("check openfsc-operator release: %w", err)
	}
	return installed, nil
}

// install runs the full standup: prerequisites, the operator chart (waiting for
// its CRDs to be Established), the Directory and the default gateways.
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

	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "creating Directory and default gateways"})
	if err := i.ensureDirectory(ctx); err != nil {
		return fmt.Errorf("ensure directory: %w", err)
	}
	if err := i.seedDefaultGateways(ctx); err != nil {
		return fmt.Errorf("seed default gateways: %w", err)
	}
	return nil
}

// ensurePrerequisites installs cert-manager and CloudNativePG (idempotent helm
// upgrade --install) and the `basic-csi` StorageClass the operator's CNPG
// Cluster references. The openfsc-operator preflights these and reports
// PrerequisitesMet=False on the Directory while they are missing.
func (i *installer) ensurePrerequisites(ctx context.Context) error {
	if err := helm.NewClient("cert-manager").InstallFromRepo(ctx, "cert-manager", certManagerChart, certManagerRepo, "", map[string]string{
		"crds.enabled": "true",
	}); err != nil {
		return fmt.Errorf("install cert-manager: %w", err)
	}

	if err := helm.NewClient("cnpg-system").InstallFromRepo(ctx, "cnpg", cnpgChart, cnpgRepo, "", nil); err != nil {
		return fmt.Errorf("install cloudnative-pg: %w", err)
	}

	return i.ensureStorageClass(ctx)
}

// ensureStorageClass creates the `basic-csi` StorageClass (an alias of the k3s
// local-path provisioner) that the operator's CNPG Cluster references.
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

// installOperator installs the vendored openfsc-operator chart (CRDs included
// in its crds/ directory) with the configured operator image.
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

// splitImageRef splits an image reference into repository and tag. The tag
// separator is the last colon after the last slash, so registry ports
// (localhost:5112/...) are kept in the repository.
func splitImageRef(ref string) (repository, tag string) {
	idx := strings.LastIndex(ref, ":")
	if idx == -1 || strings.Contains(ref[idx:], "/") {
		return ref, ""
	}
	return ref[:idx], ref[idx+1:]
}

// waitEstablished blocks until every named CRD reports the Established condition.
func waitEstablished(ctx context.Context, c client.Client, names []string) error {
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, name := range names {
			var crd apiextensionsv1.CustomResourceDefinition
			if err := c.Get(ctx, types.NamespacedName{Name: name}, &crd); err != nil {
				return false, nil //nolint:nilerr // not yet visible; keep polling
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

// ensureDirectory creates (or updates the spec of) the "default" Directory the
// operator deploys the OpenFSC core from.
func (i *installer) ensureDirectory(ctx context.Context) error {
	spec := openfscv1.DirectorySpec{
		GroupID:       i.cfg.GroupID,
		PeerID:        i.cfg.DirectoryPeerID,
		Namespace:     i.cfg.Namespace,
		ControllerURL: i.cfg.ControllerURL,
	}

	var dir openfscv1.Directory
	err := i.kube.Get(ctx, types.NamespacedName{Name: directoryName}, &dir)
	if apierrors.IsNotFound(err) {
		dir = openfscv1.Directory{
			ObjectMeta: metav1.ObjectMeta{Name: directoryName},
			Spec:       spec,
		}
		if err := i.kube.Create(ctx, &dir); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get directory: %w", err)
	}

	// Update only the plugin-owned fields; defaulted fields (postgres,
	// autoSignGrants) keep whatever the user set.
	dir.Spec.GroupID = spec.GroupID
	dir.Spec.PeerID = spec.PeerID
	dir.Spec.Namespace = spec.Namespace
	dir.Spec.ControllerURL = spec.ControllerURL
	if err := i.kube.Update(ctx, &dir); err != nil {
		return fmt.Errorf("update directory: %w", err)
	}
	return nil
}

// seedDefaultGateways creates the default Inway and Outway the plugin ships
// with if they are absent, so a fresh install comes with a registered gateway
// pair. Existing CRs are left untouched (the user may have edited them); a
// deleted default is re-seeded on the next install/upgrade.
func (i *installer) seedDefaultGateways(ctx context.Context) error {
	inway := &openfscv1.Inway{
		ObjectMeta: metav1.ObjectMeta{Name: defaultInwayName},
		Spec:       openfscv1.InwaySpec{InwayName: defaultInwayName},
	}
	if err := i.kube.Get(ctx, types.NamespacedName{Name: defaultInwayName}, &openfscv1.Inway{}); apierrors.IsNotFound(err) {
		if err := i.kube.Create(ctx, inway); err != nil {
			return fmt.Errorf("create default inway: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get default inway: %w", err)
	}

	outway := &openfscv1.Outway{
		ObjectMeta: metav1.ObjectMeta{Name: defaultOutwayName},
		Spec:       openfscv1.OutwaySpec{OutwayName: defaultOutwayName},
	}
	if err := i.kube.Get(ctx, types.NamespacedName{Name: defaultOutwayName}, &openfscv1.Outway{}); apierrors.IsNotFound(err) {
		if err := i.kube.Create(ctx, outway); err != nil {
			return fmt.Errorf("create default outway: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get default outway: %w", err)
	}
	return nil
}

// uninstall deletes the openfsc.fundament.io resources (the operator's
// finalizers tear down their helm releases and certificates), waits for them
// to be gone and then removes the operator release. Prerequisite operators
// (cert-manager, CloudNativePG) are left in place; they may be shared with
// other plugins.
func (i *installer) uninstall(ctx context.Context) error {
	for _, list := range []client.ObjectList{
		&openfscv1.InwayList{}, &openfscv1.OutwayList{}, &openfscv1.DirectoryList{}, &openfscv1.PeerList{},
	} {
		if err := i.deleteAll(ctx, list); err != nil {
			return err
		}
	}
	if err := i.waitResourcesGone(ctx); err != nil {
		return err
	}
	if err := i.operatorHelm.Uninstall(ctx, operatorRelease); err != nil {
		return fmt.Errorf("uninstall openfsc-operator: %w", err)
	}
	return nil
}

// deleteAll deletes every item in the given cluster-scoped resource list.
// A missing CRD means there is nothing to clean up.
func (i *installer) deleteAll(ctx context.Context, list client.ObjectList) error {
	if err := i.kube.List(ctx, list); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("list %T: %w", list, err)
	}
	items, err := apimetaExtractList(list)
	if err != nil {
		return err
	}
	for _, obj := range items {
		if err := i.kube.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete %s: %w", obj.GetName(), err)
		}
	}
	return nil
}

// apimetaExtractList returns a list's items as client.Objects.
func apimetaExtractList(list client.ObjectList) ([]client.Object, error) {
	items, err := apimeta.ExtractList(list)
	if err != nil {
		return nil, fmt.Errorf("extract list %T: %w", list, err)
	}
	objs := make([]client.Object, 0, len(items))
	for _, item := range items {
		obj, ok := item.(client.Object)
		if !ok {
			return nil, fmt.Errorf("unexpected list item type %T", item)
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

// waitResourcesGone waits for the operator's finalizers to finish tearing down
// all openfsc.fundament.io resources before the operator itself is removed.
func (i *installer) waitResourcesGone(ctx context.Context) error {
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		for _, list := range []client.ObjectList{
			&openfscv1.InwayList{}, &openfscv1.OutwayList{}, &openfscv1.DirectoryList{}, &openfscv1.PeerList{},
		} {
			if err := i.kube.List(ctx, list); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return false, nil //nolint:nilerr // transient; keep polling
			}
			items, err := apimetaExtractList(list)
			if err != nil {
				return false, err
			}
			if len(items) > 0 {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait for openfsc resources to be gone: %w", err)
	}
	return nil
}
