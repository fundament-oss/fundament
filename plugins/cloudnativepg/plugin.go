package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/crd"
	"github.com/fundament-oss/fundament/plugins/cloudnativepg/cnpg"
)

// healthyPhase is the status.phase CNPG reports for a Cluster that is up.
const healthyPhase = "Cluster in healthy state"

var clusterListGVK = schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"}

// catalogPath is where the image holds CloudNativePG's ClusterImageCatalog,
// fetched when the image is built (Dockerfile).
const catalogPath = "/app/image-catalog.yaml"

// setUpRetryInterval is how long Start waits before it retries a failed set-up.
const setUpRetryInterval = 30 * time.Second

// Plugin installs the CloudNativePG operator and lets project members create
// single-instance PostgreSQL databases (CNPG Clusters) from the console. It
// runs no controller of its own: the operator reconciles the Clusters, so
// databases keep running while this pod is down.
type Plugin struct {
	scheme *runtime.Scheme
	// install installs the operator: cnpg.Install, which shells out to helm,
	// replaced in tests.
	install func(context.Context, client.Client) error
	// uninstallOperator removes the operator release: cnpg.Uninstall, replaced
	// in tests.
	uninstallOperator func(context.Context) error
	catalogPath       string
	// catalog is set by Start before kube, and read by Reconcile.
	catalog []cnpg.CatalogImage
	// kube is set by Start once the operator is set up, and read by Reconcile,
	// which runs concurrently.
	kube atomic.Pointer[client.Client]
}

func newPlugin() (*Plugin, error) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apiextensionsv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return nil, fmt.Errorf("build scheme: %w", err)
		}
	}
	return &Plugin{scheme: scheme, install: cnpg.Install, uninstallOperator: cnpg.Uninstall, catalogPath: catalogPath}, nil
}

func (p *Plugin) newClient() (client.Client, error) {
	kube, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: p.scheme})
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return kube, nil
}

func (p *Plugin) Start(ctx context.Context, host pluginruntime.Host) error {
	kube, err := p.newClient()
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return pluginerrors.NewPermanent(err)
	}
	data, err := os.ReadFile(p.catalogPath)
	if err == nil {
		p.catalog, err = cnpg.ParseCatalog(data)
	}
	if err != nil {
		// A defect of the image, which a restart does not fix.
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return fmt.Errorf("read image catalog: %w", pluginerrors.NewPermanent(err))
	}

	// Retry here rather than return: the runtime ends the process on an error
	// from Start, and plugin-controller cannot read the status of a pod in a
	// crash loop. Staying up, each failure reaches the console, and the set-up
	// goes through once, for example, a release held by a killed pod is
	// cleared.
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing the CloudNativePG operator"})
	for {
		err := p.setUp(ctx, kube)
		if err == nil {
			break
		}
		host.Logger().Error("set up CloudNativePG", "error", err)
		host.ReportStatus(pluginruntime.PluginStatus{Phase: setUpFailurePhase(err), Message: err.Error()})
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(setUpRetryInterval):
		}
	}

	p.kube.Store(&kube)
	host.ReportReady()
	reportStatus(ctx, host, kube)

	<-ctx.Done()
	return nil
}

// setUp installs, upgrades or repairs the operator, then applies what the
// plugin keeps next to it: the image catalog and the user roles.
func (p *Plugin) setUp(ctx context.Context, kube client.Client) error {
	if err := p.install(ctx, kube); err != nil {
		return fmt.Errorf("install operator: %w", err)
	}
	if err := crd.VerifyAll(ctx, kube, cnpg.CRDNames); err != nil {
		return fmt.Errorf("verify CRDs: %w", err)
	}
	if err := cnpg.ApplyCatalog(ctx, kube, p.catalog); err != nil {
		return err //nolint:wrapcheck // names the catalog
	}
	if err := applyUserRoles(ctx, kube); err != nil {
		return fmt.Errorf("apply user roles: %w", err)
	}
	return nil
}

// setUpFailurePhase is the phase for a failed setUp. An operator someone else
// installed is refused for good, until they remove it. A release held by an
// operation still running is not a fault, and a retry succeeds once it is
// done.
func setUpFailurePhase(err error) pluginruntime.PluginPhase {
	switch {
	case errors.Is(err, cnpg.ErrForeignOperator):
		return pluginruntime.PhaseFailed
	case errors.Is(err, cnpg.ErrInProgress):
		return pluginruntime.PhaseInstalling
	default:
		return pluginruntime.PhaseDegraded
	}
}

func (p *Plugin) Shutdown(_ context.Context) error {
	return nil
}

// Install and Upgrade do nothing: Start installs and upgrades the operator on
// every start of the pod. The plugin implements Installer for Uninstall.
func (p *Plugin) Install(_ context.Context, _ pluginruntime.Host) error {
	return nil
}

func (p *Plugin) Upgrade(_ context.Context, _ pluginruntime.Host) error {
	return nil
}

// Uninstall refuses while databases exist: without the plugin they would keep
// running with no console page to find or delete them. Otherwise it removes
// the user roles, the image catalog and the operator release. The CRDs stay
// (the chart marks them resource-policy: keep). An operator someone else
// installed is left alone.
//
// plugin-controller skips this when the pod is unreachable, so the refusal
// only holds while the plugin is up.
func (p *Plugin) Uninstall(ctx context.Context, host pluginruntime.Host) error {
	kubePtr := p.kube.Load()
	var kube client.Client
	if kubePtr != nil {
		kube = *kubePtr
	} else {
		// Start has not finished setting up, for example while it refuses a
		// foreign operator.
		var err error
		if kube, err = p.newClient(); err != nil {
			return err
		}
	}

	names, err := databaseNames(ctx, kube)
	if err != nil {
		return err
	}
	if len(names) > 0 {
		err := fmt.Errorf("%d database(s) still exist (%s); delete them first, then uninstall the plugin", len(names), strings.Join(names, ", "))
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: "uninstall refused: " + err.Error()})
		return err
	}

	if err := deleteUserRoles(ctx, kube); err != nil {
		return err
	}
	if err := cnpg.DeleteCatalog(ctx, kube); err != nil {
		return err //nolint:wrapcheck // names the catalog
	}
	owner, err := cnpg.ForeignOwner(ctx, kube)
	if err != nil {
		return err //nolint:wrapcheck // names what it read
	}
	if owner != "" {
		return nil
	}
	if err := p.uninstallOperator(ctx); err != nil {
		return fmt.Errorf("uninstall operator: %w", err)
	}
	return nil
}

// databaseNames lists every Cluster as namespace/name. No CRD means none.
func databaseNames(ctx context.Context, kube client.Client) ([]string, error) {
	clusters := &unstructured.UnstructuredList{}
	clusters.SetGroupVersionKind(clusterListGVK)
	if err := kube.List(ctx, clusters); err != nil {
		if apimeta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list databases: %w", err)
	}
	names := make([]string, 0, len(clusters.Items))
	for i := range clusters.Items {
		names = append(names, clusters.Items[i].GetNamespace()+"/"+clusters.Items[i].GetName())
	}
	return names, nil
}

func (p *Plugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	kubePtr := p.kube.Load()
	if kubePtr == nil {
		return nil
	}
	kube := *kubePtr

	// Someone took the operator over after Start: report it, and do not fight
	// it, since every operator version change restarts every database.
	owner, err := cnpg.ForeignOwner(ctx, kube)
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: check operator owner: %w", pluginerrors.NewTransient(err))
	}
	if owner != "" {
		message := fmt.Sprintf("%s: %s; the plugin no longer repairs or upgrades it", cnpg.ErrForeignOperator, owner)
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: message})
		return nil
	}

	present, err := cnpg.OperatorPresent(ctx, kube)
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: check operator: %w", pluginerrors.NewTransient(err))
	}
	if !present {
		// The operator Deployment or a CRD was deleted. Helm still says
		// deployed, so nothing else puts them back until this pod restarts.
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "reinstalling the CloudNativePG operator"})
		if err := p.install(ctx, kube); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: setUpFailurePhase(err), Message: err.Error()})
			return fmt.Errorf("reconcile: reinstall operator: %w", pluginerrors.NewTransient(err))
		}
		if err := crd.VerifyAll(ctx, kube, cnpg.CRDNames); err != nil {
			host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
			return fmt.Errorf("reconcile: verify CRDs: %w", pluginerrors.NewTransient(err))
		}
	}
	// Puts a deleted or edited catalog back; databases referring to it do not
	// start without it.
	if err := cnpg.ApplyCatalog(ctx, kube, p.catalog); err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return fmt.Errorf("reconcile: %w", pluginerrors.NewTransient(err))
	}
	reportStatus(ctx, host, kube)
	return nil
}

// reportStatus summarizes the cluster's databases in the plugin status. The
// plugin is a capability provider: it stays Running with zero databases, and
// per-database health lives on the Cluster resources.
func reportStatus(ctx context.Context, host pluginruntime.Host, kube client.Client) {
	message, err := statusMessage(ctx, kube)
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()})
		return
	}
	host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: message})
}

func statusMessage(ctx context.Context, kube client.Client) (string, error) {
	clusters := &unstructured.UnstructuredList{}
	clusters.SetGroupVersionKind(clusterListGVK)
	if err := kube.List(ctx, clusters); err != nil {
		return "", fmt.Errorf("list databases: %w", err)
	}

	message := "CloudNativePG operator running; no databases yet"
	if n := len(clusters.Items); n > 0 {
		healthy := 0
		for i := range clusters.Items {
			phase, _, _ := unstructured.NestedString(clusters.Items[i].Object, "status", "phase")
			if phase == healthyPhase {
				healthy++
			}
		}
		noun := "databases"
		if n == 1 {
			noun = "database"
		}
		message = fmt.Sprintf("%d %s: %d healthy, %d not ready", n, noun, healthy, n-healthy)
	}

	hasDefault, err := hasDefaultStorageClass(ctx, kube)
	if err != nil {
		return "", err
	}
	if !hasDefault {
		// Not Degraded: a database can still name a StorageClass explicitly.
		message += "; no default StorageClass, pick one when creating a database"
	}
	return message, nil
}

func hasDefaultStorageClass(ctx context.Context, kube client.Client) (bool, error) {
	var classes storagev1.StorageClassList
	if err := kube.List(ctx, &classes); err != nil {
		return false, fmt.Errorf("list StorageClasses: %w", err)
	}
	for i := range classes.Items {
		annotations := classes.Items[i].Annotations
		if annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			return true, nil
		}
	}
	return false, nil
}
