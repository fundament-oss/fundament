package controller

import (
	"context"
	"fmt"
	"log/slog"
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
)

const finalizerName = "plugins.fundament.io/cleanup"

type Reconciler struct {
	client       client.Client
	logger       *slog.Logger
	cfg          config.Config
	statusPoller *statusPoller
}

// ReconcilerOption configures the Reconciler.
type ReconcilerOption func(*Reconciler)

// WithHTTPClient sets the HTTP client used for polling plugin status.
func WithHTTPClient(c connect.HTTPClient) ReconcilerOption {
	return func(r *Reconciler) {
		r.statusPoller.WithClient(c)
	}
}

func NewReconciler(c client.Client, logger *slog.Logger, cfg *config.Config, opts ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client:       c,
		logger:       logger,
		cfg:          *cfg,
		statusPoller: newStatusPoller(),
	}

	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.logger.With("plugin", req.NamespacedName)

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

	// Validate plugin name
	if err := validatePluginName(cr.Spec.PluginName); err != nil {
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

	log.Info("cleaning up plugin resources")

	// Delete ClusterRoleBindings
	for _, clusterRole := range cr.Spec.ClusterRoles {
		crbName := clusterRoleBindingName(cr.Spec.PluginName, clusterRole)
		crb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: crbName},
		}
		if err := r.client.Delete(ctx, crb); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete ClusterRoleBinding %s: %w", crbName, err)
		}
	}

	// Delete the plugin namespace — this cascades to all namespace-scoped resources
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: pluginNamespace(cr.Spec.PluginName)},
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

func (r *Reconciler) reconcileChildren(ctx context.Context, log *slog.Logger, cr *pluginsv1.PluginInstallation) error {
	fundEnvVars := r.fundamentEnvVars()
	nsName := pluginNamespace(cr.Spec.PluginName)

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
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Spec.PluginName), Namespace: nsName},
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
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Spec.PluginName), Namespace: nsName},
	}
	if op, err := controllerutil.CreateOrUpdate(ctx, r.client, rb, func() error {
		mutateRoleBinding(rb, cr)
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile RoleBinding: %w", err)
	} else if op != controllerutil.OperationResultNone {
		log.Info("reconciled resource", "kind", "RoleBinding", "name", rb.Name, "operation", op)
	}

	// ClusterRoleBindings (cluster-scoped, cleaned up via finalizer)
	// TODO: validate spec.clusterRoles against an allowlist to prevent privilege escalation.
	// Currently any user who can create a PluginInstallation CR can bind arbitrary ClusterRoles
	// (e.g. cluster-admin) to the plugin's ServiceAccount, which runs a user-specified image.
	desiredCRBs := make(map[string]struct{}, len(cr.Spec.ClusterRoles))
	for _, clusterRole := range cr.Spec.ClusterRoles {
		crbName := clusterRoleBindingName(cr.Spec.PluginName, clusterRole)
		desiredCRBs[crbName] = struct{}{}
		crb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: crbName},
		}
		if op, err := controllerutil.CreateOrUpdate(ctx, r.client, crb, func() error {
			mutateClusterRoleBinding(crb, cr, clusterRole)
			return nil
		}); err != nil {
			return fmt.Errorf("reconcile ClusterRoleBinding %s: %w", crbName, err)
		} else if op != controllerutil.OperationResultNone {
			log.Info("reconciled resource", "kind", "ClusterRoleBinding", "name", crbName, "operation", op)
		}
	}

	// Remove stale ClusterRoleBindings that are no longer in the spec
	var existingCRBs rbacv1.ClusterRoleBindingList
	if err := r.client.List(ctx, &existingCRBs, client.MatchingLabels{
		labelManagedBy: managedByValue,
		labelPlugin:    cr.Spec.PluginName,
	}); err != nil {
		return fmt.Errorf("list ClusterRoleBindings: %w", err)
	}
	for i := range existingCRBs.Items {
		crb := &existingCRBs.Items[i]
		if _, ok := desiredCRBs[crb.Name]; !ok {
			if err := r.client.Delete(ctx, crb); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete stale ClusterRoleBinding %s: %w", crb.Name, err)
			}
			log.Info("deleted stale ClusterRoleBinding", "name", crb.Name)
		}
	}

	// Deployment (in plugin namespace, no owner ref — cleaned up via namespace deletion)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Spec.PluginName), Namespace: nsName},
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
		ObjectMeta: metav1.ObjectMeta{Name: childName(cr.Spec.PluginName), Namespace: nsName},
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
