package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginerrors "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/errors"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/helpers/helm"
)

const (
	release   = "zot"
	namespace = "zot"
	repoURL   = "https://zotregistry.dev/helm-charts"
	chart     = "zot"
	// chartVersion ships zot v2.1.23.
	chartVersion = "0.1.135"
)

// chartValues override the chart defaults. The chart's default Service is a
// NodePort, which opens the registry on every node; the plugin keeps it inside
// the cluster.
var chartValues = map[string]string{
	"service.type": "ClusterIP",
}

// serviceAddress is where workloads in the cluster reach the registry.
const serviceAddress = release + "." + namespace + ".svc:5000"

// setUpRetryInterval is how long Start waits before it retries a failed set-up.
const setUpRetryInterval = 30 * time.Second

// uninstallLockWait is how long Uninstall waits for a set-up or reconcile in
// progress, which can be a `helm upgrade --wait` of up to five minutes. Past
// it, Uninstall fails and plugin-controller retries it.
const uninstallLockWait = time.Minute

// pendingGrace is how long a pending or uninstalling release counts as an
// operation in progress. Past it, the operation that held the release was most
// likely killed. It covers helm's default --wait timeout (5m) with slack.
const pendingGrace = 10 * time.Minute

// errInProgress marks an install that found another helm operation on the
// release still running, such as an earlier pod's install during a rolling
// update. It is reported as installing, not degraded, and retried.
var errInProgress = errors.New("install in progress")

// releaseClient is the part of helm.Client the plugin uses, replaced in tests.
type releaseClient interface {
	Status(ctx context.Context, releaseName string) (*helm.ReleaseStatus, error)
	InstallFromRepo(ctx context.Context, releaseName, chart, repoURL, version string, values map[string]string) error
	Uninstall(ctx context.Context, releaseName string) error
}

// Plugin installs the zot OCI registry with Helm and keeps it installed. It
// runs no controller of its own: once installed, zot keeps running while this
// pod is down. The plugin follows the operator pattern all the same: Start
// installs or repairs the release, Reconcile puts back what was deleted and
// reports zot's health, and Uninstall removes the release.
type Plugin struct {
	scheme *runtime.Scheme
	helm   releaseClient
	// newClient builds the Kubernetes client, replaced in tests.
	newClient func() (client.Client, error)
	// now is the clock for pendingGrace, replaced in tests.
	now func() time.Time
	// kube is set by Start once the release is set up, and read by Reconcile.
	// Both read and write it under lock.
	kube client.Client
	// lock serializes what changes the release: Start's set-up, Reconcile and
	// Uninstall, which all run concurrently. Without it a repair could put back
	// what an uninstall is removing. Held by sending, so a wait for it can end
	// with its context.
	lock chan struct{}
	// uninstalling is set under lock once Uninstall goes ahead. Start and
	// Reconcile then change nothing: the pod is about to go away.
	uninstalling bool
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
	p := &Plugin{
		scheme: scheme,
		helm:   helm.NewClient(namespace),
		now:    time.Now,
		lock:   make(chan struct{}, 1),
	}
	p.newClient = func() (client.Client, error) {
		kube, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: p.scheme})
		if err != nil {
			return nil, fmt.Errorf("create kubernetes client: %w", err)
		}
		return kube, nil
	}
	return p, nil
}

// acquire takes p.lock, or reports false when ctx ends first.
func (p *Plugin) acquire(ctx context.Context) bool {
	select {
	case p.lock <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (p *Plugin) release() {
	<-p.lock
}

func (p *Plugin) Start(ctx context.Context, host pluginruntime.Host) error {
	kube, err := p.newClient()
	if err != nil {
		host.ReportStatus(pluginruntime.PluginStatus{Phase: pluginruntime.PhaseFailed, Message: err.Error()})
		return pluginerrors.NewPermanent(err) //nolint:wrapcheck // newClient already names the failure; NewPermanent only marks it
	}

	// Retry here rather than return: the runtime ends the process on an error
	// from Start, and plugin-controller cannot read the status of a pod in a
	// crash loop. Staying up, each failure reaches the console.
	publish(ctx, host, kube, pluginruntime.PluginStatus{Phase: pluginruntime.PhaseInstalling, Message: "installing zot"})
	for p.setUpOnce(ctx, host, kube) != nil {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(setUpRetryInterval):
		}
	}

	<-ctx.Done()
	return nil
}

// setUpOnce is one attempt of Start's set-up, under p.lock, and reports its
// outcome. It returns nil when there is nothing left to try: the set-up
// succeeded, ctx ended, or an uninstall went ahead in the meantime.
func (p *Plugin) setUpOnce(ctx context.Context, host pluginruntime.Host, kube client.Client) error {
	if !p.acquire(ctx) {
		return nil
	}
	defer p.release()
	if p.uninstalling {
		return nil
	}
	if err := p.install(ctx, kube); err != nil {
		host.Logger().Error("install zot", "error", err)
		publish(ctx, host, kube, pluginruntime.PluginStatus{Phase: failurePhase(err), Message: err.Error()})
		return err
	}
	p.kube = kube
	host.ReportReady()
	reportStatus(ctx, host, kube)
	return nil
}

// install installs or upgrades the release to chartVersion. When the release
// is already deployed at chartVersion and the zot Deployment exists, it does
// nothing, so a plugin restart does not re-run `helm upgrade --wait`. When the
// Deployment was deleted, it re-runs the install to restore it: Helm keeps
// saying deployed, so nothing else puts it back.
func (p *Plugin) install(ctx context.Context, kube client.Client) error {
	current, err := p.helm.Status(ctx, release)
	if err != nil {
		return fmt.Errorf("read %s release: %w", release, err)
	}
	if current != nil && (current.Pending() || current.Status == "uninstalling") {
		return p.pendingError(current)
	}
	if current != nil && current.Status == "deployed" && current.ChartVersion == chartVersion {
		present, err := deploymentPresent(ctx, kube)
		if err != nil {
			return err
		}
		if present {
			return nil
		}
	}
	if err := p.helm.InstallFromRepo(ctx, release, chart, repoURL, chartVersion, chartValues); err != nil {
		if concurrentOperation(err) {
			return fmt.Errorf("%w: another install of release %s/%s started first: %w", errInProgress, namespace, release, err)
		}
		return fmt.Errorf("helm install %s: %w", chart, err)
	}
	return nil
}

// pendingError explains a pending or uninstalling release. Within pendingGrace
// it is an operation still running, which a retry resolves; after it, the
// operation was most likely killed and the release stays in that state until
// an admin clears it.
func (p *Plugin) pendingError(current *helm.ReleaseStatus) error {
	since := current.LastDeployed
	if current.Status == "uninstalling" {
		since = current.Deleted
	}
	if !since.IsZero() && p.now().Sub(since) < pendingGrace {
		return fmt.Errorf("%w: release %s/%s is %s since %s; waiting for it to finish",
			errInProgress, namespace, release, current.Status, since.UTC().Format(time.RFC3339))
	}
	recovery := fmt.Sprintf("run `helm -n %s rollback %s` to clear it", namespace, release)
	if current.Status == "uninstalling" || current.Status == "pending-install" {
		recovery = fmt.Sprintf("run `helm -n %s uninstall %s` to clear it; the plugin then installs zot again", namespace, release)
	}
	return fmt.Errorf("release %s/%s has been %s for over %s; %s", namespace, release, current.Status, pendingGrace, recovery)
}

// concurrentOperation reports whether helm refused an install because another
// operation holds the release: "another operation (install/upgrade/rollback)
// is in progress" once that operation recorded the release, or "cannot re-use
// a name that is still in use" when both started from an absent release.
func concurrentOperation(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "another operation") || strings.Contains(msg, "cannot re-use a name that is still in use")
}

// failurePhase is the phase for a failed install. A release held by an
// operation still running is not a fault: a retry succeeds once it is done.
func failurePhase(err error) pluginruntime.PluginPhase {
	if errors.Is(err, errInProgress) {
		return pluginruntime.PhaseInstalling
	}
	return pluginruntime.PhaseDegraded
}

// getDeployment reads the zot Deployment, or returns nil when it does not
// exist.
func getDeployment(ctx context.Context, kube client.Client) (*appsv1.Deployment, error) {
	var d appsv1.Deployment
	err := kube.Get(ctx, client.ObjectKey{Namespace: namespace, Name: release}, &d)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read deployment %s/%s: %w", namespace, release, err)
	}
	return &d, nil
}

func deploymentPresent(ctx context.Context, kube client.Client) (bool, error) {
	d, err := getDeployment(ctx, kube)
	return d != nil, err
}

func (p *Plugin) Shutdown(_ context.Context) error {
	return nil
}

// Install and Upgrade do nothing: Start installs and upgrades zot on every
// start of the pod. The plugin implements Installer for Uninstall.
func (p *Plugin) Install(_ context.Context, _ pluginruntime.Host) error {
	return nil
}

func (p *Plugin) Upgrade(_ context.Context, _ pluginruntime.Host) error {
	return nil
}

// Uninstall removes the console CRD and the zot release, and with the release
// every image in the registry: the chart keeps them in an emptyDir. The
// namespace stays.
//
// plugin-controller skips this when the pod is unreachable.
func (p *Plugin) Uninstall(ctx context.Context, _ pluginruntime.Host) error {
	lockCtx, cancel := context.WithTimeout(ctx, uninstallLockWait)
	defer cancel()
	if !p.acquire(lockCtx) {
		return errors.New("the plugin is still installing or repairing zot; retry the uninstall once it is done")
	}
	defer p.release()

	// From here on the uninstall goes ahead; a step that fails is retried by
	// plugin-controller, and Start and Reconcile must not undo it.
	p.uninstalling = true
	kube := p.kube
	if kube == nil {
		// Start has not finished setting up.
		var err error
		if kube, err = p.newClient(); err != nil {
			return err
		}
	}
	if err := deleteConsoleCRD(ctx, kube); err != nil {
		return err
	}
	current, err := p.helm.Status(ctx, release)
	if err != nil {
		return fmt.Errorf("read %s release: %w", release, err)
	}
	if current == nil {
		return nil
	}
	if err := p.helm.Uninstall(ctx, release); err != nil {
		return fmt.Errorf("helm uninstall %s: %w", release, err)
	}
	return nil
}

// Reconcile puts back a deleted zot Deployment and reports zot's health.
func (p *Plugin) Reconcile(ctx context.Context, host pluginruntime.Host) error {
	if !p.acquire(ctx) {
		return nil
	}
	defer p.release()
	if p.kube == nil || p.uninstalling {
		return nil
	}

	if err := p.install(ctx, p.kube); err != nil {
		publish(ctx, host, p.kube, pluginruntime.PluginStatus{Phase: failurePhase(err), Message: err.Error()})
		return fmt.Errorf("reconcile: %w", pluginerrors.NewTransient(err))
	}
	reportStatus(ctx, host, p.kube)
	return nil
}

// reportStatus reports Running while the zot Deployment has every replica
// available, and Degraded otherwise.
func reportStatus(ctx context.Context, host pluginruntime.Host, kube client.Client) {
	publish(ctx, host, kube, status(ctx, kube))
}

// publish reports status to plugin-controller and writes it, with the zot
// Deployment's state, into the console's Registry object. A console that is
// out of date must not degrade zot, so a failed write is only logged: the next
// status tries again.
func publish(ctx context.Context, host pluginruntime.Host, kube client.Client, status pluginruntime.PluginStatus) {
	host.ReportStatus(status)
	d, _ := getDeployment(ctx, kube)
	if err := syncConsole(ctx, kube, status, describeDeployment(d)); err != nil {
		host.Logger().Warn("update the console", "error", err)
	}
}

// deploymentInfo is what the console shows of the zot Deployment.
type deploymentInfo struct {
	image     string
	available string
}

func describeDeployment(d *appsv1.Deployment) deploymentInfo {
	if d == nil {
		return deploymentInfo{}
	}
	info := deploymentInfo{available: fmt.Sprintf("%d/%d", d.Status.AvailableReplicas, wantedReplicas(d))}
	if containers := d.Spec.Template.Spec.Containers; len(containers) > 0 {
		info.image = containers[0].Image
	}
	return info
}

func wantedReplicas(d *appsv1.Deployment) int32 {
	if d.Spec.Replicas != nil {
		return *d.Spec.Replicas
	}
	return 1
}

func status(ctx context.Context, kube client.Client) pluginruntime.PluginStatus {
	d, err := getDeployment(ctx, kube)
	if err != nil {
		return pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: err.Error()}
	}
	if d == nil {
		return pluginruntime.PluginStatus{Phase: pluginruntime.PhaseDegraded, Message: fmt.Sprintf("deployment %s/%s is missing", namespace, release)}
	}
	want := wantedReplicas(d)
	if d.Status.AvailableReplicas < want {
		return pluginruntime.PluginStatus{
			Phase:   pluginruntime.PhaseDegraded,
			Message: fmt.Sprintf("zot is not ready: %d of %d replicas available", d.Status.AvailableReplicas, want),
		}
	}
	return pluginruntime.PluginStatus{Phase: pluginruntime.PhaseRunning, Message: "zot is running at " + serviceAddress}
}
