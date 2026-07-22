package controller

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	// unknownDefinitionHash is the terraform provider's default placeholder,
	// used until the marketplace supplies real content hashes (FUN-11). It is
	// treated as unpinned: it can never equal a real digest, so verifying
	// against it would reject every install forever.
	unknownDefinitionHash = "sha256:unknown"

	// unknownDefinitionVersion is the terraform/console default placeholder for
	// spec.definitionRef.pluginVersion until the marketplace supplies real
	// versions (FUN-11). Unlike the hash, the version is the key used to resolve
	// the stored definition, so it cannot be silently skipped — an unpinned
	// version has nothing to fetch.
	unknownDefinitionVersion = "unknown"
)

// isUnpinned reports whether a definitionHash carries no real consent record:
// either empty or the "sha256:unknown" placeholder.
func isUnpinned(hash string) bool {
	return hash == "" || hash == unknownDefinitionHash
}

// isUnpinnedVersion reports whether a pluginVersion is the unresolved
// placeholder (empty or "unknown"). A definition is stored and fetched by its
// real metadata.version, so an unpinned version can never resolve one.
func isUnpinnedVersion(version string) bool {
	return version == "" || version == unknownDefinitionVersion
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

	// Validate installation name (used to derive every child resource name).
	if err := validateInstallationName(cr.Name); err != nil {
		cr.Status = pluginsv1.PluginInstallationStatus{
			Phase:              pluginsv1.PluginPhaseFailed,
			Message:            err.Error(),
			ObservedGeneration: cr.Generation,
		}
		_ = r.client.Status().Update(ctx, &cr)
		return ctrl.Result{}, nil //nolint:nilerr // intentional: permanent validation error, don't requeue
	}

	// Materialise child resources only when the spec actually changed. The
	// PluginDefinition is immutable and hash-pinned, and CreateOrUpdate is
	// level-based, so re-fetching the definition and re-running every child
	// mutation on the frequent status-poll requeue is pure waste. Gating on
	// ObservedGeneration keeps the fetch + child reconciliation on the (rare)
	// spec-change path while the 30s poll below only refreshes status.
	//
	// Trade-off: out-of-band drift to a child resource is corrected on the next
	// spec change or controller restart, not on the poll cadence.
	//
	// reconcileChildren mutates cr.Status.Conditions (PluginScopeReady) — persist
	// those before returning on error so the CR reflects the failure. On success
	// ObservedGeneration is advanced by the status write below (statusPoller.poll
	// stamps it), so a failed materialisation leaves it unchanged and retries.
	if cr.Status.ObservedGeneration != cr.Generation {
		if err := r.reconcileChildren(ctx, log, &cr); err != nil {
			if err := r.client.Status().Update(ctx, &cr); err != nil {
				log.Error("persist status after reconcile error failed", "err", err)
			}

			return ctrl.Result{}, fmt.Errorf("reconcile children: %w", err)
		}
	}

	// Poll plugin status and update CR. statusPoller.poll returns a fresh
	// PluginInstallationStatus with no Conditions — carry the Conditions
	// materialised inside reconcileChildren across the assignment.
	status := r.statusPoller.poll(ctx, &cr)
	status.Conditions = cr.Status.Conditions
	cr.Status = status
	if err := r.client.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	log.Info("reconciled", "phase", status.Phase)
	return ctrl.Result{RequeueAfter: r.cfg.StatusPollInterval}, nil
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

	_, err := rpcClient.RequestUninstall(ctx, connect.NewRequest(&pluginmetadatav1.RequestUninstallRequest{}))
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

	// Namespace (no owner ref — cleaned up via finalizer)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName},
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
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Name), Namespace: nsName},
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
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Name), Namespace: nsName},
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
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Name), Namespace: nsName},
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
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Name), Namespace: nsName},
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

// setPluginScopeCondition upserts the PluginScopeReady Condition on the CR's
// status. Same-value updates preserve LastTransitionTime; changes reset it.
func setPluginScopeCondition(cr *pluginsv1.PluginInstallation, status metav1.ConditionStatus, reason, message string) {
	cond := metav1.Condition{
		Type:               ConditionPluginScopeReady,
		Status:             status,
		ObservedGeneration: cr.Generation,
		Reason:             reason,
		Message:            message,
	}
	for i, existing := range cr.Status.Conditions {
		if existing.Type != ConditionPluginScopeReady {
			continue
		}
		if existing.Status == status {
			cond.LastTransitionTime = existing.LastTransitionTime
		} else {
			cond.LastTransitionTime = metav1.Now()
		}
		cr.Status.Conditions[i] = cond
		return
	}
	cond.LastTransitionTime = metav1.Now()
	cr.Status.Conditions = append(cr.Status.Conditions, cond)
}

// fetchDefinition fetches the PluginDefinition manifest from organization-api,
// verifies its sha256 against the CR's pin, and returns the parsed definition.
//
// Unpinned (empty or the "sha256:unknown" placeholder) + AllowUnpinnedHash=true
// → fetch, no comparison (dev loop).
// Unpinned + AllowUnpinnedHash=false → fail-closed error.
// Pinned → fetch and require the computed sha256 to match verbatim.
func (r *Reconciler) fetchDefinition(ctx context.Context, cr *pluginsv1.PluginInstallation) (*pluginruntime.PluginDefinition, error) {
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
		return nil, fmt.Errorf("PluginInstallation %q has no pinned spec.definitionRef.definitionHash (%q) and PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH is false", cr.Name, pinned)
	}

	// A definition is stored and fetched by its real metadata.version. The
	// "unknown" placeholder resolves nothing, so fail fast with an actionable
	// message instead of surfacing a confusing NotFound from the fetch below.
	if isUnpinnedVersion(cr.Spec.DefinitionRef.PluginVersion) {
		return nil, fmt.Errorf("PluginInstallation %q has no resolvable spec.definitionRef.pluginVersion (%q); a real published version is required to fetch its PluginDefinition (pending marketplace wiring, FUN-11)", cr.Name, cr.Spec.DefinitionRef.PluginVersion)
	}

	// Bound the RPC to keep organization-api hiccups from starving the queue.
	rpcCtx, cancel := context.WithTimeout(ctx, scopeRPCTimeout)
	defer cancel()

	got, err := r.defClient.GetDefinition(rpcCtx, cr.Spec.DefinitionRef.PluginName, cr.Spec.DefinitionRef.PluginVersion)
	if err != nil {
		return nil, fmt.Errorf("fetch definition: %w", err)
	}

	computed := pluginruntime.HashManifest(got.Manifest)
	// A pinned hash is verified verbatim. Unpinned installs (empty or the
	// "sha256:unknown" placeholder) are only reachable with AllowUnpinnedHash
	// set (checked above) and skip comparison — the dev/marketplace-pending
	// loop where no real consent hash exists yet (FUN-11).
	if !isUnpinned(pinned) && computed != pinned {
		return nil, fmt.Errorf("definition hash mismatch: pinned=%q, computed=%q", pinned, computed)
	}

	def, err := pluginruntime.ParseDefinition(got.Manifest)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
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
