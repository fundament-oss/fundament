package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	rookChartVersion = "v1.16.0"

	// fieldOwner identifies this plugin in server-side-apply managedFields.
	fieldOwner = "fundament-storage-plugin"
)

var fundamentCRDNames = []string{
	"disks.storage.fundament.io",
	"storagepools.storage.fundament.io",
}

// install runs the full install lifecycle: the rook-ceph chart, this plugin's
// CRDs, a wait for them to be established, then the CephCluster singleton.
func (p *Plugin) install(ctx context.Context, kube client.Client) error {
	if err := helm.NewClient(p.cfg.RookNamespace).InstallFromRepo(
		ctx, rookReleaseName, rookChart, rookRepoURL, rookChartVersion, RookValues(p.cfg),
	); err != nil {
		return fmt.Errorf("install rook-ceph helm chart: %w", err)
	}

	if err := applyCRDs(ctx, kube); err != nil {
		return fmt.Errorf("apply fundament CRDs: %w", err)
	}

	if err := crd.WaitEstablished(ctx, kube, fundamentCRDNames); err != nil {
		return fmt.Errorf("wait for CRDs to be established: %w", err)
	}

	if err := bootstrapCephCluster(ctx, kube, p.cfg.ClusterNamespace, p.cfg); err != nil {
		return fmt.Errorf("bootstrap CephCluster: %w", err)
	}

	return nil
}

// applyCRDs server-side applies every document in the embedded crds/ dir.
func applyCRDs(ctx context.Context, kube client.Client) error {
	entries, err := crdFS.ReadDir("crds")
	if err != nil {
		return fmt.Errorf("read embedded crds dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := crdFS.ReadFile("crds/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read embedded crd %s: %w", entry.Name(), err)
		}
		if err := applyYAMLDocs(ctx, kube, data); err != nil {
			return fmt.Errorf("apply crd %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// applyYAMLDocs server-side applies each document in a multi-document YAML.
func applyYAMLDocs(ctx context.Context, kube client.Client, data []byte) error {
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		doc, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read yaml document: %w", err)
		}
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(doc), 4096).Decode(&obj.Object); err != nil {
			return fmt.Errorf("decode yaml document: %w", err)
		}
		if obj.Object == nil {
			continue
		}

		if err := kube.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
			return fmt.Errorf("server-side apply %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

// bootstrapCephCluster creates the singleton CephCluster if absent. An existing
// one is left alone: install runs on every plugin start, and overwriting would
// clobber the spec.storage that StoragePoolReconciler maintains.
func bootstrapCephCluster(ctx context.Context, kube client.Client, namespace string, cfg Config) error {
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
