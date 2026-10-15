package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/config"
	"github.com/fundament-oss/fundament/plugin-controller/pkg/defclient"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	pluginmetadatav1 "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"
)

const (
	finalizerName = "plugins.fundament.io/cleanup"

	// scopeRPCTimeout bounds a single GetDefinition RPC to organization-api
	// during reconcile. Kept short so a wedged organization-api can't back up
	// the work queue.
	scopeRPCTimeout = 15 * time.Second

	// ConditionPluginScopeReady is the Status Condition surfaced on the CR when
	// the plugin-scope ClusterRole materialisation succeeds or fails.
	ConditionPluginScopeReady = "PluginScopeReady"

	// ConditionProgressing is True while the current spec is on its way up:
	// from the first status poll of a generation until the plugin first
	// reports Running or Degraded. Its LastTransitionTime is when that spec
	// started coming up.
	ConditionProgressing = "Progressing"

	// ConditionConfigValid is the Status Condition surfaced on the CR reporting
	// whether spec.config conforms to the pinned definition's configSchema. A
	// definition with no schema always validates (pre-schema back-compat).
	ConditionConfigValid = "ConfigValid"

	// failedRetryInterval is how long a permanently failed installation waits
	// before it is tried again. The answer rarely changes, but a version that
	// was just published, or a lookup that raced, must not stay Failed forever.
	failedRetryInterval = 2 * time.Minute

	// progressWindow bounds the fast progress poll: a spec that is still not
	// up this long after it started coming up is not about to be, and is
	// checked at the steady-state interval instead.
	progressWindow = 10 * time.Minute

	// namespaceTerminatingRetryInterval is how often an installation whose
	// namespace is still being deleted checks whether it is gone. Removing a
	// plugin namespace takes seconds to a minute.
	namespaceTerminatingRetryInterval = 5 * time.Second
)

// errNamespaceTerminating means the plugin namespace is still being deleted,
// typically by an uninstall of the same plugin just before this install. The
// installation waits for it: building in it would adopt the previous install's
// Deployment, whose pod still answers status polls as Running while the
// namespace is torn down around it.
var errNamespaceTerminating = errors.New("waiting for the previous installation's namespace to be removed")

// permanentError marks a reconcile failure that retrying the same spec cannot
// fix: a hash that does not match, a version the catalog does not have, a
// manifest that does not parse. Such an installation is reported as Failed
// rather than left without a phase, which the console can only read as an
// install that is still starting.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }

func (e *permanentError) Unwrap() error { return e.err }

func newPermanentErr(err error) error { return &permanentError{err: err} }

// isUnpinned reports whether a definitionHash carries no real consent record:
// either empty or the shared "sha256:unknown" placeholder — it can never
// equal a real digest, so verifying against it would reject every install.
func isUnpinned(hash string) bool {
	return hash == "" || hash == pluginruntime.UnknownHash
}

// hasStarted reports whether the installation's current spec has been deployed
// before, so its definition was fetched and verified at least once. The
// Progressing condition is only written after a status poll, which only runs
// once the definition checked out, and it records the generation it was
// written for: an upgrade to a version that cannot be fetched is not an
// installation that got going, even while its phase still reads Running.
//
// A generation is coarser than the pinned definition: an in-place edit of
// spec.config alone also starts a new one, and if the definition lookup then
// fails permanently (the cache emptied by a restart, and the version yanked
// since), a running plugin is marked Failed. That is an accepted edge case:
// nothing edits a PluginInstallation in place today (the terraform provider
// replaces it on any change and sets no config), so it takes a manual edit.
// Should in-place edits become common, record the deployed definitionRef in
// the status and compare against that instead.
func hasStarted(cr *pluginsv1.PluginInstallation) bool {
	if progressing := meta.FindStatusCondition(cr.Status.Conditions, ConditionProgressing); progressing != nil {
		return progressing.ObservedGeneration == cr.Generation
	}
	// A status written before the Progressing condition existed.
	if cr.Status.ObservedGeneration != cr.Generation {
		return false
	}
	switch cr.Status.Phase {
	case pluginsv1.PluginPhaseDeploying, pluginsv1.PluginPhaseRunning, pluginsv1.PluginPhaseDegraded:
		return true
	default:
		return false
	}
}

// isUnpinnedVersion reports whether a pluginVersion is the unresolved
// placeholder (empty or the shared "unknown"). A definition is stored and
// fetched by its real metadata.version, so an unpinned version can never
// resolve one.
func isUnpinnedVersion(version string) bool {
	return version == "" || version == pluginruntime.UnknownVersion
}

type Reconciler struct {
	client              client.Client
	logger              *slog.Logger
	cfg                 config.Config
	statusPoller        *statusPoller
	uninstallHTTPClient connect.HTTPClient

	// defClient fetches PluginDefinition manifests from organization-api. It
	// replaces the per-pod GetDefinition RPC: the platform DB is the source of
	// truth for the manifest, and the controller verifies its sha256 against
	// the CR's install-time consent pin before materialising RBAC or
	// launching the pod.
	defClient defclient.Client

	// defCache memoises verified, parsed PluginDefinitions by their content
	// hash so the level-triggered reconcile does not re-fetch an unchanged
	// (immutable, hash-pinned) definition from organization-api on every poll.
	defCache *definitionCache
}

// ReconcilerOption configures the Reconciler.
type ReconcilerOption func(*Reconciler)

// WithHTTPClient sets the HTTP client used for polling plugin status.
func WithHTTPClient(c connect.HTTPClient) ReconcilerOption {
	return func(r *Reconciler) {
		r.statusPoller.WithClient(c)
	}
}

// WithUninstallHTTPClient sets the HTTP client used for uninstall RPC calls.
func WithUninstallHTTPClient(c connect.HTTPClient) ReconcilerOption {
	return func(r *Reconciler) {
		r.uninstallHTTPClient = c
	}
}

// WithDefClient sets the defclient used to fetch PluginDefinition manifests
// from organization-api. Required in production; tests may inject a fake.
func WithDefClient(c defclient.Client) ReconcilerOption {
	return func(r *Reconciler) {
		r.defClient = c
	}
}

func NewReconciler(c client.Client, logger *slog.Logger, cfg *config.Config, opts ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client:              c,
		logger:              logger,
		cfg:                 *cfg,
		statusPoller:        newStatusPoller(),
		uninstallHTTPClient: &http.Client{Timeout: 5 * time.Minute},
		defCache:            newDefinitionCache(),
	}

	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.logger.With("plugin", req.Name)

	var cr pluginsv1.PluginInstallation
	if err := r.client.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get PluginInstallation: %w", err)
	}

	// Handle deletion
	if !cr.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, log, &cr)
	}

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(&cr, finalizerName) {
		controllerutil.AddFinalizer(&cr, finalizerName)
		if err := r.client.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{}, nil // re-queue with updated resource version
	}

	// Validate the installation's identity (it derives every child resource name).
	if err := validateInstallation(&cr); err != nil {
		cr.Status = pluginsv1.PluginInstallationStatus{
			Phase:              pluginsv1.PluginPhaseFailed,
			Message:            err.Error(),
			ObservedGeneration: cr.Generation,
		}
		_ = r.client.Status().Update(ctx, &cr)
		return ctrl.Result{}, nil //nolint:nilerr // intentional: permanent validation error, don't requeue
	}

	// Materialise child resources on every reconcile — the controller is
	// level-triggered, so out-of-band drift (a deleted scope ClusterRole,
	// RoleBinding or Deployment) is repaired on the next poll rather than
	// lingering until the spec changes. CreateOrUpdate is level-based, so an
	// unchanged child is a no-op read, and the immutable, hash-pinned
	// PluginDefinition is served from defCache after the first fetch, so the
	// steady-state poll neither re-fetches from organization-api nor depends on
	// it being reachable.
	//
	// reconcileChildren mutates cr.Status.Conditions (PluginScopeReady) — persist
	// those before returning on error so the CR reflects the failure.
	if err := r.reconcileChildren(ctx, log, &cr); err != nil {
		if errors.Is(err, errNamespaceTerminating) {
			cr.Status.Phase = pluginsv1.PluginPhasePending
			cr.Status.Ready = false
			cr.Status.Message = err.Error()
			if updateErr := r.client.Status().Update(ctx, &cr); updateErr != nil {
				return ctrl.Result{}, fmt.Errorf("update status: %w", updateErr)
			}
			log.Info("plugin namespace still terminating, waiting", "namespace", pluginNamespace(cr.Name))
			return ctrl.Result{RequeueAfter: namespaceTerminatingRetryInterval}, nil
		}

		// Always leave a phase behind: a CR that never gets one reads as
		// "still installing" to every client, however long it has been failing.
		var perm *permanentError
		isPermanent := errors.As(err, &perm)
		// A plugin that already got going on this spec keeps running
		// whatever this pass could not do, so keep reporting its health:
		// otherwise its phase and ready flag freeze at the last good poll, and
		// a plugin that dies meanwhile still reads Running. Read before the
		// status is touched: hasStarted falls back on ObservedGeneration.
		started := !isPermanent && hasStarted(&cr)
		switch {
		case isPermanent:
			cr.Status.Phase = pluginsv1.PluginPhaseFailed
			cr.Status.Ready = false
		case cr.Status.Phase == "":
			cr.Status.Phase = pluginsv1.PluginPhasePending
		}
		cr.Status.Message = err.Error()
		// Only claim the generation when the phase describes it. A transient
		// failure on a spec that never started leaves the previous spec's
		// phase in place, and stamping the new generation over it would make
		// the next pass take that phase for this spec's and count it started.
		if isPermanent || started {
			cr.Status.ObservedGeneration = cr.Generation
		}
		if started {
			polled := r.statusPoller.poll(ctx, &cr)
			cr.Status.Phase = polled.Phase
			cr.Status.Ready = polled.Ready
			setProgressingCondition(&cr, time.Now())
		}
		if updateErr := r.client.Status().Update(ctx, &cr); updateErr != nil {
			// Retry until the phase is saved, even for a permanent error:
			// otherwise the CR is left without one, which reads as installing.
			log.Error("persist status after reconcile error failed", "err", updateErr)
			return ctrl.Result{}, fmt.Errorf("update status after reconcile error: %w", errors.Join(err, updateErr))
		}

		if isPermanent {
			// Retrying the same spec usually gives the same answer, so skip the
			// workqueue backoff and check again at a slow, fixed pace.
			log.Error("reconcile failed permanently", "err", err)
			return ctrl.Result{RequeueAfter: failedRetryInterval}, nil
		}
		if started {
			// Retry at the steady-state poll pace rather than the workqueue
			// backoff, which grows to minutes and would let the health
			// reported above go stale just as long. Not at the faster progress
			// pace: every retry repeats the definition lookup, against a
			// service that is likely the one failing.
			log.Error("reconcile failed, plugin keeps running", "err", err)
			return ctrl.Result{RequeueAfter: r.cfg.StatusPollInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("reconcile children: %w", err)
	}

	// Poll plugin status and update CR. statusPoller.poll returns a fresh
	// PluginInstallationStatus with no Conditions — carry the Conditions
	// materialised inside reconcileChildren across the assignment.
	status := r.statusPoller.poll(ctx, &cr)
	status.Conditions = cr.Status.Conditions
	cr.Status = status
	cr.Status.Namespace = pluginNamespace(cr.Name)
	setProgressingCondition(&cr, time.Now())
	if err := r.client.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	log.Info("reconciled", "phase", status.Phase)
	return ctrl.Result{RequeueAfter: r.pollInterval(&cr, time.Now())}, nil
}

// setProgressingCondition records whether the current spec is still on its
// way up. A new generation, an install or an upgrade, starts it afresh; the
// first Running or Degraded poll of that generation settles it, and a later
// Deploying (the poller's answer for a plugin it cannot reach) leaves it
// settled.
func setProgressingCondition(cr *pluginsv1.PluginInstallation, now time.Time) {
	existing := meta.FindStatusCondition(cr.Status.Conditions, ConditionProgressing)
	switch {
	case cr.Status.Phase == pluginsv1.PluginPhaseRunning || cr.Status.Phase == pluginsv1.PluginPhaseDegraded:
		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
			Type:               ConditionProgressing,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cr.Generation,
			Reason:             "Settled",
			Message:            "the plugin has been up since this spec was applied",
		})
	case existing == nil || existing.ObservedGeneration != cr.Generation:
		// Removed first so the transition time restarts even when the
		// previous generation was still coming up too.
		meta.RemoveStatusCondition(&cr.Status.Conditions, ConditionProgressing)
		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
			Type:               ConditionProgressing,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cr.Generation,
			Reason:             "Deploying",
			Message:            "the plugin has not been up since this spec was applied",
			LastTransitionTime: metav1.NewTime(now),
		})
	}
}

// pollInterval is how long to wait before asking the plugin for its status
// again. Only the poll notices the plugin pod turning ready, so while the
// current spec is on its way up the controller asks at the faster progress
// interval, and once it has settled it drops back to the steady-state one. The
// poller also reports Deploying for a plugin it cannot reach, so a plugin that
// has been up on this spec, or one that has been coming up for longer than
// progressWindow, is treated as settled: a crash-looping plugin is not polled
// at the fast pace.
func (r *Reconciler) pollInterval(cr *pluginsv1.PluginInstallation, now time.Time) time.Duration {
	if cr.Status.Phase != pluginsv1.PluginPhaseDeploying || r.cfg.ProgressPollInterval <= 0 {
		return r.cfg.StatusPollInterval
	}
	progressing := meta.FindStatusCondition(cr.Status.Conditions, ConditionProgressing)
	if progressing == nil || progressing.Status != metav1.ConditionTrue ||
		progressing.ObservedGeneration != cr.Generation ||
		now.Sub(progressing.LastTransitionTime.Time) > progressWindow {
		return r.cfg.StatusPollInterval
	}
	return min(r.cfg.ProgressPollInterval, r.cfg.StatusPollInterval)
}

func (r *Reconciler) handleDeletion(ctx context.Context, log *slog.Logger, cr *pluginsv1.PluginInstallation) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(cr, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Update status to Terminating
	cr.Status.Phase = pluginsv1.PluginPhaseTerminating
	cr.Status.Message = "uninstalling plugin"
	_ = r.client.Status().Update(ctx, cr)

	// Request plugin uninstall before tearing down resources
	if result, err := r.requestPluginUninstall(ctx, log, cr); err != nil || !result.IsZero() {
		return result, err
	}

	log.Info("cleaning up plugin resources")

	// Delete the plugin-scope ClusterRole/ClusterRoleBinding materialised from
	// the pinned PluginDefinition (may not exist if the reconciler never
	// succeeded past the fetch/verify step).
	scopeName := pluginScopeClusterRoleName(cr.Name)
	scopeCRB := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: scopeName}}
	if err := r.client.Delete(ctx, scopeCRB); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete plugin-scope ClusterRoleBinding %s: %w", scopeName, err)
	}
	scopeRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: scopeName}}
	if err := r.client.Delete(ctx, scopeRole); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete plugin-scope ClusterRole %s: %w", scopeName, err)
	}

	// Delete the plugin namespace — this cascades to all namespace-scoped resources
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: pluginNamespace(cr.Name)},
	}
	if err := r.client.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete Namespace: %w", err)
	}

	controllerutil.RemoveFinalizer(cr, finalizerName)
	if err := r.client.Update(ctx, cr); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
	}

	log.Info("finalizer cleanup complete")
	return ctrl.Result{}, nil
}

func (r *Reconciler) requestPluginUninstall(ctx context.Context, log *slog.Logger, cr *pluginsv1.PluginInstallation) (ctrl.Result, error) {
	url := pluginServiceURL(cr.Name)
	rpcClient := pluginmetadatav1connect.NewPluginMetadataServiceClient(r.uninstallHTTPClient, url)

	_, err := rpcClient.RequestUninstall(ctx, &pluginmetadatav1.RequestUninstallRequest{})
	if err != nil {
		// If the plugin is unreachable, proceed with cleanup
		if connect.CodeOf(err) == connect.CodeUnavailable || connect.CodeOf(err) == connect.CodeUnimplemented {
			log.Info("plugin unreachable or does not support uninstall, proceeding with cleanup", "error", err)
			return ctrl.Result{}, nil
		}

		// For other RPC errors, requeue to retry
		log.Error("plugin uninstall failed, will retry", "error", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, fmt.Errorf("request plugin uninstall: %w", err)
	}

	log.Info("plugin uninstall completed")
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileChildren(ctx context.Context, log *slog.Logger, cr *pluginsv1.PluginInstallation) error {
	fundEnvVars := r.fundamentEnvVars()
	nsName := pluginNamespace(cr.Name)

	// Fetch + verify + parse the PluginDefinition first. The manifest is the
	// source of truth for both the scope RBAC (feeding the ClusterRole) and
	// the container image (feeding the Deployment). Doing this up front means
	// a hash mismatch aborts before any RBAC or Pod is materialised.
	def, err := r.fetchDefinition(ctx, cr)
	if err != nil {
		setPluginScopeCondition(cr, metav1.ConditionFalse, "MaterialisationFailed", err.Error())
		return fmt.Errorf("reconcile plugin scope: %w", err)
	}

	// Enforce the definition's configSchema before materialising anything: an
	// invalid spec.config fails closed exactly like a hash mismatch, and is
	// permanent — the same spec revalidates to the same answer. The definition
	// is the hash-verified one, so what is enforced is what the admin
	// consented to.
	if err := pluginruntime.ValidateConfig(cr.Spec.Config, def.Spec.ConfigSchema); err != nil {
		setCondition(cr, ConditionConfigValid, metav1.ConditionFalse, "ConfigInvalid", err.Error())
		return newPermanentErr(fmt.Errorf("validate config: %w", err))
	}
	setCondition(cr, ConditionConfigValid, metav1.ConditionTrue, "Validated",
		"spec.config conforms to the definition's configSchema")

	// Namespace (no owner ref — cleaned up via finalizer)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName},
	}
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(ns), ns); err == nil {
		if !ns.DeletionTimestamp.IsZero() {
			return errNamespaceTerminating
		}
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get Namespace: %w", err)
	}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, ns, func() error {
		mutateNamespace(ns, cr)
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile Namespace: %w", err)
	} else if op != controllerutil.OperationResultNone {
		log.Info("reconciled resource", "kind", "Namespace", "name", ns.Name, "operation", op)
	}

	// ServiceAccount (in plugin namespace, no owner ref — cleaned up via namespace deletion)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: childResourceName, Namespace: nsName},
	}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, sa, func() error {
		mutateServiceAccount(sa, cr)
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile ServiceAccount: %w", err)
	} else if op != controllerutil.OperationResultNone {
		log.Info("reconciled resource", "kind", "ServiceAccount", "name", sa.Name, "operation", op)
	}

	// RoleBinding (in plugin namespace, no owner ref — cleaned up via namespace deletion)
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: childResourceName, Namespace: nsName},
	}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, rb, func() error {
		mutateRoleBinding(rb, cr)
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile RoleBinding: %w", err)
	} else if op != controllerutil.OperationResultNone {
		log.Info("reconciled resource", "kind", "RoleBinding", "name", rb.Name, "operation", op)
	}

	// Materialise the plugin-scope ClusterRole (FUN-17) from the fetched
	// PluginDefinition. The manifest is the source of truth for the plugin's
	// declared permissions; the CR's DefinitionHash binds admin consent to
	// this specific manifest.
	//
	// Errors here are surfaced via the PluginScopeReady Condition AND
	// returned so the workqueue retries with exponential backoff. Without the
	// retry, a transient failure would leave the SA with no scope until
	// the informer's periodic resync (~10h).
	if err := r.reconcilePluginScope(ctx, log, cr, def); err != nil {
		setPluginScopeCondition(cr, metav1.ConditionFalse, "MaterialisationFailed", err.Error())
		return fmt.Errorf("reconcile plugin scope: %w", err)
	}
	// Record readiness. An unpinned definition gets a distinct reason so the CR
	// advertises that its RBAC carries no hash consent.
	if isUnpinned(cr.Spec.DefinitionRef.DefinitionHash) {
		setPluginScopeCondition(cr, metav1.ConditionTrue, "MaterialisedUnpinned",
			"plugin-scope ClusterRole materialised from an UNPINNED definition (no hash consent); RBAC reflects whatever the manifest declared")
	} else {
		setPluginScopeCondition(cr, metav1.ConditionTrue, "Materialised", "plugin-scope ClusterRole materialised from PluginDefinition manifest")
	}

	// Deployment (in plugin namespace, no owner ref — cleaned up via namespace deletion).
	// Sourced AFTER the scope: the plugin pod must never observe an SA whose
	// scope hasn't been reconciled yet.
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: childResourceName, Namespace: nsName},
	}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, deploy, func() error {
		mutateDeployment(deploy, cr, def, fundEnvVars)
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile Deployment: %w", err)
	} else if op != controllerutil.OperationResultNone {
		log.Info("reconciled resource", "kind", "Deployment", "name", deploy.Name, "operation", op)
	}

	// Service (in plugin namespace, no owner ref — cleaned up via namespace deletion)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: childResourceName, Namespace: nsName},
	}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, svc, func() error {
		mutateService(svc, cr)
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile Service: %w", err)
	} else if op != controllerutil.OperationResultNone {
		log.Info("reconciled resource", "kind", "Service", "name", svc.Name, "operation", op)
	}

	return nil
}

// setCondition upserts a Condition of the given type on the CR's status.
// Same-value updates preserve LastTransitionTime; changes reset it.
func setCondition(cr *pluginsv1.PluginInstallation, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: cr.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// setPluginScopeCondition upserts the PluginScopeReady Condition on the CR.
func setPluginScopeCondition(cr *pluginsv1.PluginInstallation, status metav1.ConditionStatus, reason, message string) {
	setCondition(cr, ConditionPluginScopeReady, status, reason, message)
}

// fetchDefinition fetches the PluginDefinition manifest from organization-api,
// verifies its sha256 against the CR's pin, and returns the parsed definition.
//
// Unpinned (empty or the "sha256:unknown" placeholder) + AllowUnpinnedHash=true
// → fetch, no comparison (dev loop).
// Unpinned + AllowUnpinnedHash=false → fail-closed error.
// Pinned → fetch and require the computed sha256 to match verbatim.
func (r *Reconciler) fetchDefinition(ctx context.Context, cr *pluginsv1.PluginInstallation) (*pluginruntime.PluginDefinition, error) {
	def, err := r.fetchDefinitionOnce(ctx, cr)
	var perm *permanentError
	if errors.As(err, &perm) && hasStarted(cr) {
		// An installation whose current spec already got going had its
		// definition fetched and verified before. A lookup that now fails (a yanked version, changed
		// catalog bytes, a tightened hash policy, or the in-memory cache emptied
		// by a restart) must not flip a healthy plugin to Failed: keep retrying.
		return nil, perm.err
	}
	return def, err
}

func (r *Reconciler) fetchDefinitionOnce(ctx context.Context, cr *pluginsv1.PluginInstallation) (*pluginruntime.PluginDefinition, error) {
	if r.defClient == nil {
		// Guards a misconfigured construction (NewReconciler without
		// WithDefClient): fail with a clear error instead of a nil-panic.
		return nil, fmt.Errorf("plugin-controller misconfigured: no definition client (WithDefClient) set")
	}

	pinned := cr.Spec.DefinitionRef.DefinitionHash
	if isUnpinned(pinned) && !r.cfg.AllowUnpinnedHash {
		// Fail-closed: a CR without a real pin cannot materialise arbitrary RBAC.
		// The operator opts into unpinned installs (empty or the "sha256:unknown"
		// placeholder) by setting PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH=true on
		// the Deployment.
		return nil, newPermanentErr(fmt.Errorf("PluginInstallation %q has no pinned spec.definitionRef.definitionHash (%q) and PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH is false", cr.Name, pinned))
	}

	// A definition is stored and fetched by its real metadata.version. The
	// "unknown" placeholder resolves nothing, so fail fast with an actionable
	// message instead of surfacing a confusing NotFound from the fetch below.
	if isUnpinnedVersion(cr.Spec.DefinitionRef.PluginVersion) {
		return nil, newPermanentErr(fmt.Errorf("PluginInstallation %q has no resolvable spec.definitionRef.pluginVersion (%q); a real published version is required to fetch its PluginDefinition (pending marketplace wiring, FUN-11)", cr.Name, cr.Spec.DefinitionRef.PluginVersion))
	}

	// A pinned definition is immutable and content-addressed, so a previously
	// verified manifest for this hash can be served from cache — the
	// level-triggered reconcile hits this on every poll without touching
	// organization-api. Unpinned dev installs have no stable hash and fall
	// through to a fresh fetch each time (hot-reload).
	if !isUnpinned(pinned) && r.defCache != nil {
		if def, ok := r.defCache.get(pinned); ok {
			return def, nil
		}
	}

	// Bound the RPC to keep organization-api hiccups from starving the queue.
	rpcCtx, cancel := context.WithTimeout(ctx, scopeRPCTimeout)
	defer cancel()

	got, err := r.defClient.GetDefinition(rpcCtx, cr.Spec.DefinitionRef.OrganizationName, cr.Spec.DefinitionRef.PluginName, cr.Spec.DefinitionRef.PluginVersion)
	if err != nil {
		err = fmt.Errorf("fetch definition: %w", err)
		// The catalog does not have this version, or refuses the reference
		// outright: asking again soon will not change that. Anything else, an
		// unreachable catalog included, is worth another try right away.
		if code := connect.CodeOf(err); code == connect.CodeNotFound || code == connect.CodeInvalidArgument {
			return nil, newPermanentErr(err)
		}
		return nil, err
	}

	computed := pluginruntime.HashManifest(got.Manifest)
	// A pinned hash is verified verbatim. Unpinned installs (empty or the
	// "sha256:unknown" placeholder) are only reachable with AllowUnpinnedHash
	// set (checked above) and skip comparison — the dev/marketplace-pending
	// loop where no real consent hash exists yet (FUN-11).
	if !isUnpinned(pinned) && computed != pinned {
		return nil, newPermanentErr(fmt.Errorf("definition hash mismatch: pinned=%q, computed=%q", pinned, computed))
	}

	def, err := pluginruntime.ParseDefinition(got.Manifest)
	if err != nil {
		return nil, newPermanentErr(fmt.Errorf("parse manifest: %w", err))
	}

	// Cache only verified, pinned definitions — keyed by the consent hash so a
	// republish under a new hash is a cache miss, never a stale hit.
	if !isUnpinned(pinned) && r.defCache != nil {
		r.defCache.put(pinned, &def)
	}
	return &def, nil
}

// reconcilePluginScope materialises the plugin-scope ClusterRole + binding from
// the parsed PluginDefinition. The definition itself is fetched + verified
// upstream in fetchDefinition.
func (r *Reconciler) reconcilePluginScope(ctx context.Context, log *slog.Logger, cr *pluginsv1.PluginInstallation, def *pluginruntime.PluginDefinition) error {
	scopeRoleName := pluginScopeClusterRoleName(cr.Name)
	scopeRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: scopeRoleName}}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, scopeRole, func() error {
		mutatePluginScopeClusterRole(scopeRole, cr, def.Spec.Permissions.RBAC)
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile plugin-scope ClusterRole: %w", err)
	} else if op != controllerutil.OperationResultNone {
		log.Info("reconciled resource", "kind", "ClusterRole", "name", scopeRoleName, "operation", op)
	}

	scopeCRB := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: scopeRoleName}}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, scopeCRB, func() error {
		mutatePluginScopeClusterRoleBinding(scopeCRB, cr)
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile plugin-scope ClusterRoleBinding: %w", err)
	} else if op != controllerutil.OperationResultNone {
		log.Info("reconciled resource", "kind", "ClusterRoleBinding", "name", scopeRoleName, "operation", op)
	}

	return nil
}

func (r *Reconciler) fundamentEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "FUNDAMENT_CLUSTER_ID", Value: r.cfg.FundamentClusterID},
		{Name: "FUNDAMENT_INSTALL_ID", Value: r.cfg.FundamentInstallID},
		{Name: "FUNDAMENT_ORGANIZATION_ID", Value: r.cfg.FundamentOrgID},
	}
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&pluginsv1.PluginInstallation{}).
		Complete(r)

	if err != nil {
		return fmt.Errorf("setup controller: %w", err)
	}

	return nil
}

// RequeueAfter returns the configured status poll interval for use in tests.
func (r *Reconciler) RequeueAfter() time.Duration {
	return r.cfg.StatusPollInterval
}
