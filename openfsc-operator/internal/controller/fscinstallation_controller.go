//go:generate controller-gen rbac:roleName=openfsc-operator paths=. output:rbac:dir=../../config/rbac

package controller

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
	"github.com/fundament-oss/fundament/openfsc-operator/charts"
	"github.com/fundament-oss/fundament/openfsc-operator/internal/helm"
)

const (
	resyncInterval        = 5 * time.Minute
	gatewayResyncInterval = time.Minute
	pendingRetryInterval  = 15 * time.Second
)

const installationFinalizer = "openfsc.fundament.io/fscinstallation"

// prereqCRDs are the third-party CRDs an installation needs. The operator
// never installs other operators: it reports PrerequisitesMet=False and waits
// for cert-manager / CloudNativePG to be installed out-of-band.
var prereqCRDs = []string{
	"certificates.cert-manager.io",
	"issuers.cert-manager.io",
	"clusters.postgresql.cnpg.io",
}

type FSCInstallationReconciler struct {
	Client client.Client // cached: FSCInstallation, Deployments
	Direct client.Client // uncached: CRD preflight, Certificates, Secrets, CNPG
	Admin  *AdminClients
}

func (r *FSCInstallationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openfscv1.FSCInstallation{}).
		Named("fscinstallation").
		Complete(r); err != nil {
		return fmt.Errorf("setup fscinstallation controller: %w", err)
	}
	return nil
}

// The rules cover what the reconciler touches directly plus everything the
// installed Helm charts render; chart/templates/rbac.yaml is the deployable
// ClusterRole and config/rbac/role.yaml the generated reference.
//
// +kubebuilder:rbac:groups=openfsc.fundament.io,resources=fscinstallations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=openfsc.fundament.io,resources=fscinstallations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openfsc.fundament.io,resources=fscinstallations/finalizers,verbs=update
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets;services;serviceaccounts;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete

func (r *FSCInstallationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var inst openfscv1.FSCInstallation
	if err := r.Client.Get(ctx, req.NamespacedName, &inst); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get FSCInstallation: %w", err)
	}

	if !inst.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(&inst, installationFinalizer) {
			older, err := r.olderSibling(ctx, &inst)
			if err != nil {
				return ctrl.Result{}, err
			}
			// Only the namespace's owner tears down: a conflict loser never
			// provisioned anything, and the fixed names mean tearing down on its
			// behalf would destroy the older sibling's workloads.
			if older == "" {
				if err := r.teardown(ctx, &inst); err != nil {
					inst.Status.Phase = openfscv1.PhaseError
					inst.Status.Message = fmt.Sprintf("teardown: %v", err)
					r.setCondition(&inst, openfscv1.ConditionReady, metav1.ConditionFalse, "TeardownError", err.Error())
					_ = r.Client.Status().Update(ctx, &inst)
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(&inst, installationFinalizer)
			if err := r.Client.Update(ctx, &inst); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	before := inst.Status.DeepCopy()
	result, err := r.reconcile(ctx, &inst)
	if err != nil {
		inst.Status.Phase = openfscv1.PhaseError
		inst.Status.Message = err.Error()
		r.setCondition(&inst, openfscv1.ConditionReady, metav1.ConditionFalse, "ReconcileError", err.Error())
	}
	if !apiequality.Semantic.DeepEqual(before, &inst.Status) {
		if uerr := r.Client.Status().Update(ctx, &inst); uerr != nil {
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("%w (additionally, update status: %w)", err, uerr)
			}
			return ctrl.Result{}, fmt.Errorf("update status: %w", uerr)
		}
	}
	return result, err
}

func (r *FSCInstallationReconciler) reconcile(ctx context.Context, inst *openfscv1.FSCInstallation) (ctrl.Result, error) {
	if conflict, err := r.olderSibling(ctx, inst); err != nil {
		return ctrl.Result{}, err
	} else if conflict != "" {
		msg := fmt.Sprintf("namespace %s already hosts FSCInstallation %s; this resource is ignored", inst.Namespace, conflict)
		inst.Status.Phase = openfscv1.PhaseError
		inst.Status.Message = msg
		r.setCondition(inst, openfscv1.ConditionReady, metav1.ConditionFalse, "Conflict", msg)
		return ctrl.Result{RequeueAfter: pendingRetryInterval}, nil
	}

	// The finalizer is added only after winning the namespace, so deleting a
	// conflict loser stays a no-op.
	if controllerutil.AddFinalizer(inst, installationFinalizer) {
		if err := r.Client.Update(ctx, inst); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Unreachable while the CRD's CEL rules are enforced; a stale CRD from an
	// older install would otherwise let this panic the values builders.
	if inst.Spec.Directory.External != nil && inst.Spec.Certificate == nil {
		msg := "spec.certificate is required for directory.mode External"
		inst.Status.Phase = openfscv1.PhaseError
		inst.Status.Message = msg
		r.setCondition(inst, openfscv1.ConditionCertificatesReady, metav1.ConditionFalse, "CertificateMissing", msg)
		r.setCondition(inst, openfscv1.ConditionReady, metav1.ConditionFalse, "InvalidSpec", msg)
		return ctrl.Result{RequeueAfter: pendingRetryInterval}, nil
	}

	missing, err := r.missingPrereqCRDs(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("check prerequisite CRDs: %w", err)
	}
	if len(missing) > 0 {
		msg := fmt.Sprintf("waiting for prerequisite CRDs: %s (install cert-manager and CloudNativePG)", strings.Join(missing, ", "))
		r.setCondition(inst, openfscv1.ConditionPrerequisitesMet, metav1.ConditionFalse, "MissingCRDs", msg)
		return r.pending(inst, msg), nil
	}
	r.setCondition(inst, openfscv1.ConditionPrerequisitesMet, metav1.ConditionTrue, "Present", "cert-manager and CloudNativePG CRDs are present")

	if inst.Spec.Directory.External != nil {
		missingSecrets, err := r.missingSecrets(ctx, inst)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(missingSecrets) > 0 {
			msg := fmt.Sprintf("waiting for referenced Secrets: %s", strings.Join(missingSecrets, ", "))
			r.setCondition(inst, openfscv1.ConditionCertificatesReady, metav1.ConditionFalse, "SecretsMissing", msg)
			return r.pending(inst, msg), nil
		}
		r.setCondition(inst, openfscv1.ConditionCertificatesReady, metav1.ConditionTrue, "SecretsPresent", "referenced certificate Secrets are present")
	}

	helmClient := helm.NewClient(inst.Namespace)

	if err := r.ensureCore(ctx, inst, helmClient); err != nil {
		return ctrl.Result{}, err
	}

	if inst.Spec.Directory.Mode == openfscv1.DirectoryModeSelf {
		certReady, err := r.managerCertReady(ctx, inst)
		if err != nil {
			return ctrl.Result{}, err
		}
		if certReady {
			r.setCondition(inst, openfscv1.ConditionCertificatesReady, metav1.ConditionTrue, "Issued", "group CA and Manager group certificate are issued")
		} else {
			r.setCondition(inst, openfscv1.ConditionCertificatesReady, metav1.ConditionFalse, "Issuing", "waiting for cert-manager to issue the group certificates")
		}
	}

	inst.Status.ManagerAddress = managerAddress(inst)
	inst.Status.ControllerURL = inst.Spec.ControllerURL

	coreReady, coreDetail, err := r.deploymentsReady(ctx, inst.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	reg := registrations{}
	if coreReady {
		if reg, err = r.observeRegistrations(ctx, inst); err != nil {
			return ctrl.Result{}, err
		}
	}

	inways, outways, gatewaysActive, err := r.ensureGateways(ctx, inst, helmClient, reg)
	if err != nil {
		return ctrl.Result{}, err
	}
	inst.Status.Inways = inways
	inst.Status.Outways = outways
	// Advanced only here, once ensureCore and ensureGateways have applied this
	// generation's spec; provisionGateway reads the prior value above to detect
	// an outdated release, so an early return must leave it untouched.
	inst.Status.ObservedGeneration = inst.Generation

	if !coreReady {
		return r.pending(inst, coreDetail), nil
	}
	if !gatewaysActive {
		return r.pending(inst, "OpenFSC core is running; waiting for gateways to register with the Controller"), nil
	}

	msg := "OpenFSC Manager and Controller are running"
	requeue := resyncInterval
	if n := len(inst.Spec.Inways) + len(inst.Spec.Outways); n > 0 {
		msg = fmt.Sprintf("%s; all %d gateways are registered", msg, n)
		requeue = gatewayResyncInterval
	}
	inst.Status.Phase = openfscv1.PhaseActive
	inst.Status.Message = msg
	r.setCondition(inst, openfscv1.ConditionReady, metav1.ConditionTrue, "Running", msg)
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (r *FSCInstallationReconciler) ensureCore(ctx context.Context, inst *openfscv1.FSCInstallation, helmClient *helm.Client) error {
	deployedVersion, err := helmClient.DeployedChartVersion(umbrellaRelease)
	if err != nil {
		return fmt.Errorf("check umbrella release: %w", err)
	}
	deployed := meta.FindStatusCondition(inst.Status.Conditions, openfscv1.ConditionCoreDeployed)
	if deployed != nil && deployed.Status == metav1.ConditionTrue &&
		deployed.ObservedGeneration == inst.Generation && deployedVersion == charts.Version {
		return nil
	}

	if err := applyCoreResources(ctx, r.Direct, inst); err != nil {
		return fmt.Errorf("apply core resources: %w", err)
	}
	umbrella, err := loadUmbrellaChart()
	if err != nil {
		return err
	}
	if err := helmClient.UpgradeInstall(ctx, umbrellaRelease, umbrella, coreValues(inst)); err != nil {
		return fmt.Errorf("install OpenFSC umbrella: %w", err)
	}
	// A redeploy may reissue the controller's internal Secret; drop any cached
	// Administration API client built from the old one.
	r.Admin.forget(inst.Namespace)

	r.setCondition(inst, openfscv1.ConditionCoreDeployed, metav1.ConditionTrue, "Applied", "core resources and OpenFSC umbrella applied")
	return nil
}

// teardown removes everything the installation provisioned. The team's
// namespace itself is never touched.
func (r *FSCInstallationReconciler) teardown(ctx context.Context, inst *openfscv1.FSCInstallation) error {
	helmClient := helm.NewClient(inst.Namespace)
	for _, prefix := range []string{inwayRelease(""), outwayRelease("")} {
		releases, err := helmClient.List(prefix)
		if err != nil {
			return fmt.Errorf("list gateway releases: %w", err)
		}
		for _, release := range releases {
			if release.ChartName != gatewayChartName(release.Name) {
				continue
			}
			if err := helmClient.Uninstall(release.Name); err != nil {
				return fmt.Errorf("uninstall gateway release %s: %w", release.Name, err)
			}
			if err := deleteGatewayCerts(ctx, r.Direct, inst.Namespace, release.Name); err != nil {
				return err
			}
		}
	}
	if err := r.uninstallUmbrella(helmClient); err != nil {
		return err
	}
	if err := deleteCoreResources(ctx, r.Direct, inst); err != nil {
		return err
	}
	r.Admin.forget(inst.Namespace)
	return nil
}

// uninstallUmbrella removes the umbrella release, but only when it was
// installed from the open-fsc chart — release names are fixed, so an
// unrelated user release that happens to be named "fsc" must survive.
func (r *FSCInstallationReconciler) uninstallUmbrella(helmClient *helm.Client) error {
	releases, err := helmClient.List(umbrellaRelease)
	if err != nil {
		return fmt.Errorf("list umbrella release: %w", err)
	}
	for _, release := range releases {
		if release.Name != umbrellaRelease || release.ChartName != umbrellaChartName {
			continue
		}
		if err := helmClient.Uninstall(release.Name); err != nil {
			return fmt.Errorf("uninstall umbrella: %w", err)
		}
	}
	return nil
}

// olderSibling returns the name of an older FSCInstallation in the same
// namespace, if any. The oldest resource (ties broken by name) owns the
// namespace; the operator supports one installation per namespace because all
// component names in it are fixed.
func (r *FSCInstallationReconciler) olderSibling(ctx context.Context, inst *openfscv1.FSCInstallation) (string, error) {
	var list openfscv1.FSCInstallationList
	if err := r.Client.List(ctx, &list, client.InNamespace(inst.Namespace)); err != nil {
		return "", fmt.Errorf("list FSCInstallations: %w", err)
	}
	for i := range list.Items {
		sib := &list.Items[i]
		if sib.Name == inst.Name {
			continue
		}
		if sib.CreationTimestamp.Before(&inst.CreationTimestamp) ||
			(sib.CreationTimestamp.Equal(&inst.CreationTimestamp) && sib.Name < inst.Name) {
			return sib.Name, nil
		}
	}
	return "", nil
}

func (r *FSCInstallationReconciler) missingSecrets(ctx context.Context, inst *openfscv1.FSCInstallation) ([]string, error) {
	names := map[string]bool{inst.Spec.Directory.External.TrustAnchor.Name: true}
	if inst.Spec.Certificate != nil {
		names[inst.Spec.Certificate.ExistingSecret] = true
	}
	for _, gw := range inst.Spec.Inways {
		if gw.Certificate != nil {
			names[gw.Certificate.ExistingSecret] = true
		}
	}
	for _, gw := range inst.Spec.Outways {
		if gw.Certificate != nil {
			names[gw.Certificate.ExistingSecret] = true
		}
	}

	var missing []string
	for name := range names {
		var secret corev1.Secret
		err := r.Direct.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: name}, &secret)
		if apierrors.IsNotFound(err) {
			missing = append(missing, name)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get secret %s: %w", name, err)
		}
	}
	slices.Sort(missing)
	return missing, nil
}

func (r *FSCInstallationReconciler) managerCertReady(ctx context.Context, inst *openfscv1.FSCInstallation) (bool, error) {
	got := &unstructured.Unstructured{}
	got.SetAPIVersion("cert-manager.io/v1")
	got.SetKind("Certificate")
	err := r.Direct.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: managerGroupCertSecret}, got)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get manager group certificate: %w", err)
	}
	return certReady(got), nil
}

func (r *FSCInstallationReconciler) missingPrereqCRDs(ctx context.Context) ([]string, error) {
	var missing []string
	for _, name := range prereqCRDs {
		var crd apiextensionsv1.CustomResourceDefinition
		err := r.Direct.Get(ctx, types.NamespacedName{Name: name}, &crd)
		if apierrors.IsNotFound(err) {
			missing = append(missing, name)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get CRD %s: %w", name, err)
		}
	}
	return missing, nil
}

func (r *FSCInstallationReconciler) deploymentsReady(ctx context.Context, ns string) (bool, string, error) {
	for _, name := range []string{managerDeployment, controllerDeployment} {
		var deploy appsv1.Deployment
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &deploy)
		if apierrors.IsNotFound(err) {
			return false, fmt.Sprintf("waiting for Deployment %s/%s", ns, name), nil
		}
		if err != nil {
			return false, "", fmt.Errorf("get Deployment %s/%s: %w", ns, name, err)
		}
		if !deploymentAvailable(&deploy) {
			return false, fmt.Sprintf("Deployment %s/%s not yet Available", ns, name), nil
		}
	}
	return true, "", nil
}

func (r *FSCInstallationReconciler) pending(inst *openfscv1.FSCInstallation, msg string) ctrl.Result {
	inst.Status.Phase = openfscv1.PhasePending
	inst.Status.Message = msg
	r.setCondition(inst, openfscv1.ConditionReady, metav1.ConditionFalse, "Pending", msg)
	return ctrl.Result{RequeueAfter: pendingRetryInterval}
}

func (r *FSCInstallationReconciler) setCondition(inst *openfscv1.FSCInstallation, condType string, status metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: inst.Generation,
		Reason:             reason,
		Message:            msg,
	})
}

func deploymentAvailable(deploy *appsv1.Deployment) bool {
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
