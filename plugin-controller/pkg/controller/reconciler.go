package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
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
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	finalizerName = "plugins.fundament.io/cleanup"

	// scopeRPCTimeout bounds a single GetDefinition RPC during reconcile.
	// Kept short so one wedged plugin pod can't back up the work queue.
	scopeRPCTimeout = 15 * time.Second

	// ConditionPluginScopeReady is the Status Condition surfaced on the CR when
	// the plugin-scope ClusterRole materialisation succeeds or fails.
	ConditionPluginScopeReady = "PluginScopeReady"

	// unknownDefinitionHash is the terraform provider's default placeholder,
	// used until the marketplace supplies real content hashes (FUN-11). It is
	// treated as unpinned: it can never equal a real digest, so verifying
	// against it would reject every install forever.
	unknownDefinitionHash = "sha256:unknown"
)

// isUnpinned reports whether a definitionHash carries no real consent record:
// either empty or the "sha256:unknown" placeholder.
func isUnpinned(hash string) bool {
	return hash == "" || hash == unknownDefinitionHash
}

type Reconciler struct {
	client              client.Client
	logger              *slog.Logger
	cfg                 config.Config
	statusPoller        *statusPoller
	uninstallHTTPClient connect.HTTPClient
	// scopeHTTPClient is used for the per-reconcile GetDefinition RPC. Distinct
	// from uninstallHTTPClient (5m) because a slow or wedged plugin pod would
	// otherwise starve the reconcile work queue.
	scopeHTTPClient connect.HTTPClient

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

// WithUninstallHTTPClient sets the HTTP client used for uninstall RPC calls.
func WithUninstallHTTPClient(c connect.HTTPClient) ReconcilerOption {
	return func(r *Reconciler) {
		r.uninstallHTTPClient = c
	}
}

// WithScopeHTTPClient sets the HTTP client used for the per-reconcile
// GetDefinition RPC that materialises the plugin-scope ClusterRole.
func WithScopeHTTPClient(c connect.HTTPClient) ReconcilerOption {
	return func(r *Reconciler) {
		r.scopeHTTPClient = c
	}
}

func NewReconciler(c client.Client, logger *slog.Logger, cfg *config.Config, opts ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client:              c,
		logger:              logger,
		cfg:                 *cfg,
		statusPoller:        newStatusPoller(),
		uninstallHTTPClient: &http.Client{Timeout: 5 * time.Minute},
		scopeHTTPClient:     &http.Client{Timeout: scopeRPCTimeout},
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

	// Reconcile child resources. reconcileChildren mutates cr.Status.Conditions
	// (PluginScopeReady) — persist those before returning on error so the CR
	// reflects the failure, even though we'll requeue.

	if err := r.reconcileChildren(ctx, log, &cr); err != nil {
		if err := r.client.Status().Update(ctx, &cr); err != nil {
			log.Error("persist status after reconcile error failed", "err", err)
		}

		return ctrl.Result{}, fmt.Errorf("reconcile children: %w", err)
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
	// PluginMetadataService/GetDefinition. The plugin itself is the source of
	// truth for its declared permissions; admins pin
	// cr.Spec.DefinitionRef.DefinitionHash to bind consent to a specific
	// definition shape.
	//
	// Errors here are surfaced via a PluginScopeReady Condition on the CR AND
	// returned so the workqueue retries with exponential backoff. Without the
	// retry, a transient RPC failure would leave the SA with no scope until
	// the informer's periodic resync (~10h).
	// reconcilePluginScope sets the success PluginScopeReady condition itself
	// (Materialised / MaterialisedUnpinned); we only record the failure here.
	if err := r.reconcilePluginScope(ctx, log, cr); err != nil {
		setPluginScopeCondition(cr, metav1.ConditionFalse, "MaterialisationFailed", err.Error())
		return fmt.Errorf("reconcile plugin scope: %w", err)
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

// reconcilePluginScope RPCs the plugin's GetDefinition, verifies the pinned
// hash, materialises the scope ClusterRole + binding, and sets the success
// PluginScopeReady condition on the CR (the caller records failures).
func (r *Reconciler) reconcilePluginScope(ctx context.Context, log *slog.Logger, cr *pluginsv1.PluginInstallation) error {
	pinned := cr.Spec.DefinitionRef.DefinitionHash
	unpinned := isUnpinned(pinned)
	if unpinned && !r.cfg.AllowUnpinnedHash {
		// Fail-closed: a CR without a real pin cannot materialise arbitrary RBAC.
		// The operator opts into unpinned installs (empty or the "sha256:unknown"
		// placeholder) by setting PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH=true on
		// the Deployment.
		return fmt.Errorf("PluginInstallation %q has no pinned spec.definitionRef.definitionHash (%q) and PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH is false", cr.Name, pinned)
	}

	url := r.pluginServiceURLOverride
	if url == "" {
		url = pluginServiceURL(cr.Name)
	}
	// Bound the RPC to keep one wedged plugin from starving the reconcile queue.
	rpcCtx, cancel := context.WithTimeout(ctx, scopeRPCTimeout)
	defer cancel()
	rpcClient := pluginmetadatav1connect.NewPluginMetadataServiceClient(r.scopeHTTPClient, url)
	resp, err := rpcClient.GetDefinition(rpcCtx, connect.NewRequest(&pluginmetadatav1.GetDefinitionRequest{}))
	if err != nil {
		return fmt.Errorf("GetDefinition RPC: %w", err)
	}
	def := resp.Msg

	if !unpinned {
		got, err := hashDefinition(def)
		if err != nil {
			return fmt.Errorf("hash definition: %w", err)
		}
		if got != pinned {
			return fmt.Errorf("definition hash mismatch: pinned=%q, plugin=%q", pinned, got)
		}
	} else {
		// Unpinned install — only reachable with AllowUnpinnedHash set. We
		// materialise whatever RBAC the plugin's live GetDefinition returns with
		// no consent bound to a definition hash, so a compromised or updated
		// plugin image could widen its own ClusterRole. Log loudly so this is
		// auditable; the PluginScopeReady condition below also carries an
		// "unpinned/unverified" reason.
		log.Warn("materialising plugin-scope RBAC from an UNPINNED definition; no hash consent enforced (PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH is set)",
			"plugin", cr.Name, "definitionHash", pinned)
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

	// Materialisation succeeded — record readiness. An unpinned definition gets a
	// distinct reason so the CR advertises that its RBAC carries no hash consent.
	if unpinned {
		setPluginScopeCondition(cr, metav1.ConditionTrue, "MaterialisedUnpinned",
			"plugin-scope ClusterRole materialised from an UNPINNED definition (no hash consent); RBAC reflects whatever the plugin returned")
	} else {
		setPluginScopeCondition(cr, metav1.ConditionTrue, "Materialised", "plugin-scope ClusterRole materialised from GetDefinition")
	}
	return nil
}

// hashDefinition computes SHA-256 over a canonical JSON serialization of the
// plugin's GetDefinitionResponse. Protojson gives a stable text form; walking
// it through encoding/json.Marshal after Unmarshal into a generic map sorts
// object keys deterministically. Unlike proto.MarshalOptions{Deterministic:
// true} — which the protobuf-go documentation explicitly warns is NOT stable
// across library versions or languages — this scheme produces the same bytes
// for the same logical message across controller upgrades.
func hashDefinition(def *pluginmetadatav1.GetDefinitionResponse) (string, error) {
	// UseProtoNames keeps field names as declared in the .proto file (snake_case).
	raw, err := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: false}.Marshal(def)
	if err != nil {
		return "", fmt.Errorf("protojson marshal: %w", err)
	}
	canonical, err := canonicalizeJSON(raw)
	if err != nil {
		return "", fmt.Errorf("canonicalize json: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// canonicalizeJSON round-trips JSON through a sorted intermediate to produce a
// byte-stable representation: object keys sorted lexicographically, arrays kept
// in order (already ordered by protojson), no insignificant whitespace.
func canonicalizeJSON(raw []byte) ([]byte, error) {
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return marshalSorted(tree)
}

func marshalSorted(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var buf []byte
		buf = append(buf, '{')
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vb, err := marshalSorted(t[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		buf = append(buf, '}')
		return buf, nil
	case []any:
		var buf []byte
		buf = append(buf, '[')
		for i, e := range t {
			if i > 0 {
				buf = append(buf, ',')
			}
			eb, err := marshalSorted(e)
			if err != nil {
				return nil, err
			}
			buf = append(buf, eb...)
		}
		buf = append(buf, ']')
		return buf, nil
	default:
		return json.Marshal(v)
	}
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
