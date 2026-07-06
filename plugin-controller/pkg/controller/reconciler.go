package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	pluginmetadatav1 "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"
	"google.golang.org/protobuf/proto"
)

const (
	finalizerName = "plugins.fundament.io/cleanup"

	// devHashSentinel is the reserved DefinitionHash value that bypasses the
	// reconciler's hash-verification step. Local dev uses this so the CR
	// stays valid under the CRD's `startsWith("sha256:")` rule without
	// computing a real content hash.
	devHashSentinel = "sha256:mock"
)

type Reconciler struct {
	client              client.Client
	logger              *slog.Logger
	cfg                 config.Config
	statusPoller        *statusPoller
	uninstallHTTPClient connect.HTTPClient

	// pluginServiceURLOverride, when set, replaces pluginServiceURL(cr.Name)
	// in RPC calls. Used by tests to point the reconciler at a local stub.
	pluginServiceURLOverride string
}

// ReconcilerOption configures the Reconciler.
type ReconcilerOption func(*Reconciler)

// WithHTTPClient sets the HTTP client used for polling plugin status.
func WithHTTPClient(c connect.HTTPClient) ReconcilerOption {
	return func(r *Reconciler) {
		r.statusPoller.WithClient(c)
	}
}

// WithUninstallHTTPClient sets the HTTP client used for uninstall RPC calls
// and for the definition-fetch RPC during reconcile.
func WithUninstallHTTPClient(c connect.HTTPClient) ReconcilerOption {
	return func(r *Reconciler) {
		r.uninstallHTTPClient = c
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

	// Reconcile child resources
	if err := r.reconcileChildren(ctx, log, &cr); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile children: %w", err)
	}

	// Poll plugin status and update CR
	status := r.statusPoller.poll(ctx, &cr)
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

	// Delete legacy spec.ClusterRoles bindings.
	for _, roleName := range cr.Spec.ClusterRoles {
		crbName := legacyClusterRoleBindingName(cr.Name, roleName)
		crb := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: crbName}}
		if err := r.client.Delete(ctx, crb); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete legacy ClusterRoleBinding %s: %w", crbName, err)
		}
	}

	// Delete the plugin-scope ClusterRole/ClusterRoleBinding materialised from
	// the plugin's declared definition (may not exist if the plugin never
	// answered GetDefinition).
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

	// Legacy spec.ClusterRoles bindings — restored so plugins whose runtime
	// needs cluster-wide perms at startup (e.g. helm-installing operators) can
	// come up before the FUN-17 scope ClusterRole is materialised. Empty list
	// is fine; the definition-driven scope below still runs.
	for _, roleName := range cr.Spec.ClusterRoles {
		crbName := legacyClusterRoleBindingName(cr.Name, roleName)
		crb := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: crbName}}
		if op, err := controllerutil.CreateOrUpdate(ctx, r.client, crb, func() error {
			mutateLegacyClusterRoleBinding(crb, cr, roleName)
			return nil
		}); err != nil {
			return fmt.Errorf("reconcile legacy ClusterRoleBinding %s: %w", crbName, err)
		} else if op != controllerutil.OperationResultNone {
			log.Info("reconciled resource", "kind", "ClusterRoleBinding", "name", crbName, "operation", op)
		}
	}

	// Deployment (in plugin namespace, no owner ref — cleaned up via namespace deletion)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Name), Namespace: nsName},
	}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, deploy, func() error {
		mutateDeployment(deploy, cr, fundEnvVars)
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

	// Materialise the plugin-scope ClusterRole (FUN-17) from the plugin's own
	// PluginMetadataService/GetDefinition. Best-effort: if the plugin pod
	// isn't Ready yet, the RPC fails and the next reconcile tick retries.
	// The plugin itself is the source of truth for its declared permissions;
	// admins pin cr.Spec.DefinitionRef.DefinitionHash to bind consent to a
	// specific definition shape.
	if err := r.reconcilePluginScope(ctx, log, cr); err != nil {
		log.Warn("plugin-scope ClusterRole not (yet) materialised", "err", err)
	}

	return nil
}

// reconcilePluginScope RPCs the plugin's GetDefinition, verifies the pinned
// hash if set, and materialises the scope ClusterRole + binding.
func (r *Reconciler) reconcilePluginScope(ctx context.Context, log *slog.Logger, cr *pluginsv1.PluginInstallation) error {
	url := r.pluginServiceURLOverride
	if url == "" {
		url = pluginServiceURL(cr.Name)
	}
	rpcClient := pluginmetadatav1connect.NewPluginMetadataServiceClient(r.uninstallHTTPClient, url)
	resp, err := rpcClient.GetDefinition(ctx, connect.NewRequest(&pluginmetadatav1.GetDefinitionRequest{}))
	if err != nil {
		return fmt.Errorf("GetDefinition RPC: %w", err)
	}
	def := resp.Msg

	// Hash verification — the admin's install-time consent record. The literal
	// "sha256:mock" (and empty, when the CRD relaxes to omit it) is a reserved
	// sentinel that bypasses the check; local dev sets it so the CR stays
	// valid without computing a real hash.
	if pinned := cr.Spec.DefinitionRef.DefinitionHash; pinned != "" && pinned != devHashSentinel {
		got, err := hashDefinition(def)
		if err != nil {
			return fmt.Errorf("hash definition: %w", err)
		}
		if got != pinned {
			return fmt.Errorf("definition hash mismatch: pinned=%q, plugin=%q", pinned, got)
		}
	}

	scopeRoleName := pluginScopeClusterRoleName(cr.Name)
	scopeRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: scopeRoleName}}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, scopeRole, func() error {
		mutatePluginScopeClusterRole(scopeRole, cr, def.GetPermissions().GetRbac())
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

// hashDefinition computes SHA-256 over the canonical (deterministic) proto
// serialization of the plugin's GetDefinitionResponse. Deterministic mode
// gives the same bytes for the same message every time — the admin can pin a
// stable hash across restarts and controllers can enforce it.
func hashDefinition(def *pluginmetadatav1.GetDefinitionResponse) (string, error) {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(def)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
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
