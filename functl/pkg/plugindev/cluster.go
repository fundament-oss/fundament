package plugindev

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	crdName          = "plugininstallations.plugins.fundament.io"
	controllerSelect = "app.kubernetes.io/component=plugin-controller"
)

var (
	crdGVR          = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	installationGVR = schema.GroupVersionResource{Group: "plugins.fundament.io", Version: "v1", Resource: "plugininstallations"}
)

type kubeCluster struct {
	target  Target
	typed   kubernetes.Interface
	dynamic dynamic.Interface
}

// NewCluster connects to t.KubeContext using the default kubeconfig rules.
func NewCluster(t Target) (Cluster, error) {
	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: t.KubeContext},
	)

	// ClientConfig alone would not tell a missing context from other errors.
	raw, err := cfg.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	if _, ok := raw.Contexts[t.KubeContext]; !ok {
		return nil, &CheckError{
			Problem: fmt.Sprintf("kube context %q not found in your kubeconfig", t.KubeContext),
			Fix:     "create the sandbox cluster with `just plugins cluster-create`",
			DocsURL: t.DocsURL,
		}
	}

	rest, err := cfg.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build kube client config: %w", err)
	}
	typed, err := kubernetes.NewForConfig(rest)
	if err != nil {
		return nil, fmt.Errorf("create kube client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(rest)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	return &kubeCluster{target: t, typed: typed, dynamic: dyn}, nil
}

func (k *kubeCluster) Reachable(ctx context.Context) error {
	if err := k.typed.Discovery().RESTClient().Get().AbsPath("/version").Do(ctx).Error(); err != nil {
		return fmt.Errorf("server version: %w", err)
	}
	return nil
}

func (k *kubeCluster) CRDHasDevField(ctx context.Context) (bool, error) {
	crd, err := k.dynamic.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get CRD %s: %w", crdName, err)
	}
	versions, _, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if err != nil {
		return false, fmt.Errorf("read CRD versions: %w", err)
	}
	return servedVersionHasDev(versions), nil
}

// servedVersionHasDev reports whether the served installationGVR version of a CRD has spec.dev.
func servedVersionHasDev(versions []any) bool {
	for _, v := range versions {
		vm, ok := v.(map[string]any)
		if !ok || vm["name"] != installationGVR.Version || vm["served"] != true {
			continue
		}
		_, found, err := unstructured.NestedMap(vm, "schema", "openAPIV3Schema", "properties", "spec", "properties", "dev")
		return err == nil && found
	}
	return false
}

func (k *kubeCluster) ControllerEnv(ctx context.Context) (map[string]string, error) {
	list, err := k.typed.AppsV1().Deployments(k.target.ControllerNamespace).List(ctx, metav1.ListOptions{LabelSelector: controllerSelect})
	if err != nil {
		return nil, fmt.Errorf("list controller deployments: %w", err)
	}
	if len(list.Items) == 0 || len(list.Items[0].Spec.Template.Spec.Containers) == 0 {
		return nil, &CheckError{
			Problem: fmt.Sprintf("no plugin controller deployment found in namespace %q", k.target.ControllerNamespace),
			Fix:     deployFix,
			DocsURL: k.target.DocsURL,
		}
	}
	env := map[string]string{}
	for _, e := range list.Items[0].Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	return env, nil
}

func (k *kubeCluster) Installation(ctx context.Context, name string) (InstallationState, bool, error) {
	obj, err := k.dynamic.Resource(installationGVR).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return InstallationState{}, false, nil
	}
	if err != nil {
		return InstallationState{}, false, fmt.Errorf("get PluginInstallation %q: %w", name, err)
	}
	version, _, err := unstructured.NestedString(obj.Object, "spec", "definitionRef", "pluginVersion")
	if err != nil {
		return InstallationState{}, false, fmt.Errorf("read pluginVersion of %q: %w", name, err)
	}
	return InstallationState{Version: version, Terminating: obj.GetDeletionTimestamp() != nil}, true, nil
}

func (k *kubeCluster) DeleteInstallation(ctx context.Context, name string) error {
	err := k.dynamic.Resource(installationGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete PluginInstallation %q: %w", name, err)
	}
	return nil
}
