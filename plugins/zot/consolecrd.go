package main

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
)

// The console shows zot in the organization sidebar through the cluster-scoped
// CRD in crds/. The plugin keeps one Registry object, named after the release,
// and writes the status it reports into it.
const (
	consoleCRDName   = "registries.zot.fundament.io"
	fieldOwner       = "zot-plugin"
	establishTimeout = 15 * time.Second
)

var registryGVK = schema.GroupVersionKind{Group: "zot.fundament.io", Version: "v1alpha1", Kind: "Registry"}

//go:embed crds/zot.fundament.io_registries.yaml
var consoleCRDYAML []byte

// waitEstablished is a variable so that tests can skip the wait: the fake
// client never sets the Established condition.
var waitEstablished = crd.WaitEstablished

// syncConsole applies the console CRD and the Registry object, and writes
// status and the zot Deployment's state into it. The wait for the CRD is
// bounded, so a CRD that never becomes Established cannot hold up a Reconcile
// for long.
func syncConsole(ctx context.Context, kube client.Client, status pluginruntime.PluginStatus, info deploymentInfo) error {
	crdObj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(consoleCRDYAML, &crdObj.Object); err != nil {
		return fmt.Errorf("decode console CRD: %w", err)
	}
	if err := kube.Apply(ctx, client.ApplyConfigurationFromUnstructured(crdObj), client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return fmt.Errorf("apply %s: %w", consoleCRDName, err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, establishTimeout)
	defer cancel()
	if err := waitEstablished(waitCtx, kube, []string{consoleCRDName}); err != nil {
		return fmt.Errorf("wait for %s: %w", consoleCRDName, err)
	}

	registry := newRegistry()
	registry.Object["spec"] = map[string]any{"chartVersion": chartVersion}
	if err := kube.Apply(ctx, client.ApplyConfigurationFromUnstructured(registry), client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return fmt.Errorf("apply registry %s: %w", release, err)
	}

	withStatus := newRegistry()
	withStatus.Object["status"] = map[string]any{
		"phase":     string(status.Phase),
		"message":   status.Message,
		"address":   serviceAddress,
		"image":     info.image,
		"available": info.available,
	}
	if err := kube.Status().Apply(ctx, client.ApplyConfigurationFromUnstructured(withStatus), client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return fmt.Errorf("write registry %s status: %w", release, err)
	}
	return nil
}

func newRegistry() *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{}}
	u.SetGroupVersionKind(registryGVK)
	u.SetName(release)
	return u
}

// deleteConsoleCRD deletes the console CRD, and with it the Registry object,
// so the console sidebar no longer lists zot.
func deleteConsoleCRD(ctx context.Context, kube client.Client) error {
	obj := &apiextensionsv1.CustomResourceDefinition{}
	obj.SetName(consoleCRDName)
	if err := client.IgnoreNotFound(kube.Delete(ctx, obj)); err != nil {
		return fmt.Errorf("delete %s: %w", consoleCRDName, err)
	}
	return nil
}
