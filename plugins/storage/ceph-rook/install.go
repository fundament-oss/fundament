package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

//go:embed crds/*.yaml
var crdFS embed.FS

const (
	rookReleaseName = "rook-ceph"
	rookChart       = "rook-ceph"
	rookRepoURL     = "https://charts.rook.io/release"

	cephClusterFieldOwner = "fundament-storage-plugin"
)

var fundamentCRDNames = []string{
	"disks.storage.fundament.io",
	"storagepools.storage.fundament.io",
}

// install performs the full install lifecycle:
//  1. Installs the rook-ceph operator Helm chart.
//  2. Server-side applies the plugin's own CRDs (Disk, StoragePool).
//  3. Waits for those CRDs to be established.
//  4. Server-side applies the singleton CephCluster bootstrap object
//     (creates on first run; does not overwrite spec.storage on subsequent runs).
func (p *Plugin) install(ctx context.Context, kube client.Client) error {
	// Step 1: Install the rook-ceph operator via Helm.
	if err := helm.NewClient(p.cfg.RookNamespace).InstallFromRepo(
		ctx, rookReleaseName, rookChart, rookRepoURL, p.cfg.RookChartVersion, RookValues(p.cfg),
	); err != nil {
		return fmt.Errorf("install rook-ceph helm chart: %w", err)
	}

	// Step 2: Apply the plugin's own CRDs.
	if err := applyCRDs(ctx, kube); err != nil {
		return fmt.Errorf("apply fundament CRDs: %w", err)
	}

	// Step 3: Wait for the CRDs to be established.
	if err := waitEstablished(ctx, kube, fundamentCRDNames); err != nil {
		return fmt.Errorf("wait for CRDs to be established: %w", err)
	}

	// Step 4: Bootstrap the CephCluster singleton.
	if err := bootstrapCephCluster(ctx, kube, p.cfg.ClusterNamespace, p.cfg); err != nil {
		return fmt.Errorf("bootstrap CephCluster: %w", err)
	}

	return nil
}

// applyCRDs reads all YAML files from the embedded crds/ directory and
// server-side applies each document as an unstructured object.
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

// applyYAMLDocs splits a multi-document YAML byte slice into individual
// documents and server-side applies each one.
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

		if err := kube.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.ForceOwnership, client.FieldOwner(cephClusterFieldOwner)); err != nil {
			return fmt.Errorf("server-side apply %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

// bootstrapCephCluster creates the singleton CephCluster if it does not exist.
// If it already exists, it is left untouched so that spec.storage (disk
// assignments) managed by the Task 9/10 reconciler are not overwritten.
func bootstrapCephCluster(ctx context.Context, kube client.Client, namespace string, cfg Config) error {
	desired := BootstrapCephCluster(namespace, cfg)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(desired.GroupVersionKind())
	err := kube.Get(ctx, types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if err == nil {
		// Already exists — do not overwrite spec.storage.
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get CephCluster: %w", err)
	}

	// Plain create closes the TOCTOU race: if the object appeared between the
	// Get above and this Create, an AlreadyExists result means someone else
	// created it — we must not touch its spec.storage.
	if err := kube.Create(ctx, desired); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil // created concurrently — do not touch its spec.storage
		}
		return fmt.Errorf("create CephCluster: %w", err)
	}
	return nil
}

// waitEstablished polls until every named CRD reports Established=True.
// Copied from plugins/openfsc/installer.go.
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
