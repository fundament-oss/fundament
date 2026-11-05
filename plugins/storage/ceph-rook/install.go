package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

//go:embed crds/*.yaml
var crdFS embed.FS

const (
	rookReleaseName = "rook-ceph"
	rookChart       = "rook-ceph"
	rookRepoURL     = "https://charts.rook.io/release"

	// Not a config field: the reconcilers are written against this chart's CRDs,
	// so it belongs to the build. Pinning it here means the digest-pinned image —
	// and the manifest hash an admin consents to — binds the operator too.
	// The plugin installs only this version; see checkRookVersion.
	rookChartVersion = "v1.21.1"

	// From Rook v1.20 the rook-ceph chart installs ceph-csi-operator but no CSI
	// driver: this second chart declares the drivers. Its version is the
	// ceph-csi-operator subchart's in rookChartVersion.
	csiDriversReleaseName  = "ceph-csi-drivers"
	csiDriversChart        = "ceph-csi-drivers"
	csiDriversRepoURL      = "https://ceph.github.io/ceph-csi-operator"
	csiDriversChartVersion = "1.1.0"

	// fieldOwner identifies this plugin in server-side-apply managedFields.
	fieldOwner = "fundament-storage-plugin"
)

// rookCRDNames are the Rook kinds the manager starts informers for. The
// CephBlockPool watch in DiskPoolReconciler.SetupWithManager syncs its cache
// at manager start, so a CRD the chart has applied but the API server has not
// established yet fails the sync and takes the whole manager down.
var rookCRDNames = []string{
	"cephclusters.ceph.rook.io",
	"cephblockpools.ceph.rook.io",
	"cephfilesystems.ceph.rook.io",
	"cephobjectstores.ceph.rook.io",
}

// install runs the full install lifecycle: the rook-ceph and ceph-csi-drivers
// charts, this plugin's CRDs, a wait for them to be established, then the
// CephCluster singleton.
func (p *Plugin) install(ctx context.Context, kube client.Client) error {
	h := helm.NewClient(p.cfg.RookNamespace)
	if err := p.checkRookVersion(ctx, kube, h); err != nil {
		return err
	}
	if err := h.InstallFromRepo(ctx, rookReleaseName, rookChart, rookRepoURL, rookChartVersion, RookValues(&p.cfg)); err != nil {
		return fmt.Errorf("install rook-ceph helm chart: %w", err)
	}
	if err := h.InstallFromRepo(ctx, csiDriversReleaseName, csiDriversChart, csiDriversRepoURL, csiDriversChartVersion, CSIDriversValues(&p.cfg)); err != nil {
		return fmt.Errorf("install ceph-csi-drivers helm chart: %w", err)
	}

	if err := crd.WaitEstablished(ctx, kube, rookCRDNames); err != nil {
		return fmt.Errorf("wait for rook CRDs to be established: %w", err)
	}

	fundamentCRDs, err := applyCRDs(ctx, kube)
	if err != nil {
		return fmt.Errorf("apply fundament CRDs: %w", err)
	}

	if err := crd.WaitEstablished(ctx, kube, fundamentCRDs); err != nil {
		return fmt.Errorf("wait for CRDs to be established: %w", err)
	}

	if err := bootstrapCephCluster(ctx, kube, p.cfg.ClusterNamespace, &p.cfg); err != nil {
		return fmt.Errorf("bootstrap CephCluster: %w", err)
	}

	return nil
}

// checkRookVersion refuses to install over a Rook other than rookChartVersion:
// Rook upgrades one minor version at a time, so installing the pin over an
// older cluster could break its storage. Rook's CRDs outlive its release, so
// with no release the running CephCluster's daemons tell the version.
func (p *Plugin) checkRookVersion(ctx context.Context, kube client.Client, h *helm.Client) error {
	current, err := h.Status(ctx, rookReleaseName)
	if err != nil {
		return fmt.Errorf("read rook-ceph release: %w", err)
	}
	var daemonVersions []string
	clusterExists := false
	if current == nil || current.Status == "uninstalled" {
		clusterExists, daemonVersions, err = runningCluster(ctx, kube, p.cfg.ClusterNamespace)
		if err != nil {
			return err
		}
	}
	return incompatibleRook(current, clusterExists, daemonVersions)
}

// incompatibleRookHint is how a refusal ends.
const incompatibleRookHint = "this plugin installs Rook " + rookChartVersion +
	" only on a cluster without Rook or with that version; see 8b-3 in docs/developer/plugins/ceph-rook-runbook.md"

// incompatibleRook returns an error when the release (nil when absent) or,
// without one, a running CephCluster is at another Rook version.
func incompatibleRook(current *helm.ReleaseStatus, clusterExists bool, daemonVersions []string) error {
	if current != nil && current.Status != "uninstalled" {
		if !isRookChartVersion(current.ChartVersion) {
			return fmt.Errorf("the Helm release %s is at Rook %s: %s", rookReleaseName, current.ChartVersion, incompatibleRookHint)
		}
		return nil
	}
	if !clusterExists {
		return nil
	}
	if len(daemonVersions) == 0 {
		return fmt.Errorf("a CephCluster runs without a Helm release %s and no daemon carries a %s label: %s",
			rookReleaseName, rookVersionLabel, incompatibleRookHint)
	}
	for _, v := range daemonVersions {
		if !isRookChartVersion(v) {
			return fmt.Errorf("a CephCluster runs Rook %s without a Helm release %s: %s", v, rookReleaseName, incompatibleRookHint)
		}
	}
	return nil
}

// isRookChartVersion reports whether v names rookChartVersion. Rook's chart
// repository publishes some versions without the leading v, and helm records
// the one it resolved.
func isRookChartVersion(v string) bool {
	return strings.TrimPrefix(v, "v") == strings.TrimPrefix(rookChartVersion, "v")
}

// runningCluster reports whether a CephCluster exists in namespace, and the
// rookVersionLabel of each Rook daemon Deployment there.
func runningCluster(ctx context.Context, kube client.Client, namespace string) (bool, []string, error) {
	clusters := &unstructured.UnstructuredList{}
	clusters.SetGroupVersionKind(schema.GroupVersionKind{Group: "ceph.rook.io", Version: "v1", Kind: "CephClusterList"})
	if err := kube.List(ctx, clusters, client.InNamespace(namespace)); err != nil {
		if apimeta.IsNoMatchError(err) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("list CephClusters: %w", err)
	}
	if len(clusters.Items) == 0 {
		return false, nil, nil
	}
	var daemons appsv1.DeploymentList
	if err := kube.List(ctx, &daemons, client.InNamespace(namespace), client.HasLabels{rookVersionLabel}); err != nil {
		return false, nil, fmt.Errorf("list Rook daemons: %w", err)
	}
	versions := make([]string, 0, len(daemons.Items))
	for i := range daemons.Items {
		versions = append(versions, daemons.Items[i].Labels[rookVersionLabel])
	}
	return true, versions, nil
}

// rookVersionLabel is the label Rook sets on every Ceph daemon Deployment: the
// operator version that last deployed it.
const rookVersionLabel = "rook-version"

// applyCRDs server-side applies every document in the embedded crds/ dir and
// returns the applied names, so the establishment wait can never miss a CRD
// added to the directory later (an applied-but-unwaited CRD races its informer
// at manager start, exactly the failure the rookCRDNames comment warns about).
func applyCRDs(ctx context.Context, kube client.Client) ([]string, error) {
	entries, err := crdFS.ReadDir("crds")
	if err != nil {
		return nil, fmt.Errorf("read embedded crds dir: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := crdFS.ReadFile("crds/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read embedded crd %s: %w", entry.Name(), err)
		}
		applied, err := applyYAMLDocs(ctx, kube, data)
		if err != nil {
			return nil, fmt.Errorf("apply crd %s: %w", entry.Name(), err)
		}
		names = append(names, applied...)
	}
	return names, nil
}

// applyYAMLDocs server-side applies each document in a multi-document YAML and
// returns the applied object names.
func applyYAMLDocs(ctx context.Context, kube client.Client, data []byte) ([]string, error) {
	var names []string
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read yaml document: %w", err)
		}
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(doc), 4096).Decode(&obj.Object); err != nil {
			return nil, fmt.Errorf("decode yaml document: %w", err)
		}
		if obj.Object == nil {
			continue
		}

		if err := kube.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
			return nil, fmt.Errorf("server-side apply %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
		names = append(names, obj.GetName())
	}
	return names, nil
}

// bootstrapCephCluster creates the singleton CephCluster if absent. An existing
// one is left alone: install runs on every plugin start, and overwriting would
// clobber the spec.storage that DiskPoolReconciler maintains.
func bootstrapCephCluster(ctx context.Context, kube client.Client, namespace string, cfg *Config) error {
	desired := BootstrapCephCluster(namespace, cfg)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(desired.GroupVersionKind())
	err := kube.Get(ctx, types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get CephCluster: %w", err)
	}

	// Plain create closes the TOCTOU race: AlreadyExists means someone else got
	// there, so leave its spec.storage alone.
	if err := kube.Create(ctx, desired); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil // created concurrently — do not touch its spec.storage
		}
		return fmt.Errorf("create CephCluster: %w", err)
	}
	return nil
}
