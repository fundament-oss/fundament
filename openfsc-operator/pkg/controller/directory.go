package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/fundament-oss/fundament/openfsc-operator/charts"
	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/pkg/api/v1"
	"github.com/fundament-oss/fundament/openfsc-operator/pkg/helm"
)

// OpenFSC umbrella release and Deployment names (release "shared",
// fullnameOverride=shared so the umbrella's internal-TLS certificate
// CommonNames/SANs match the subchart service names).
const (
	umbrellaRelease      = "shared"
	managerDeployment    = "shared-open-fsc-manager"
	controllerDeployment = "shared-open-fsc-controller"
)

const (
	// resyncInterval is the steady-state requeue once a resource is Active.
	resyncInterval = 5 * time.Minute
	// pendingRetryInterval requeues while the directory comes up (or its
	// prerequisites are still missing).
	pendingRetryInterval = 15 * time.Second
)

// directoryFinalizer guarantees the umbrella release, the directory
// prerequisites and the self Peer are removed before the Directory disappears.
const directoryFinalizer = "openfsc.fundament.io/directory"

// selfPeerName is the name of the operator-owned Peer representing the
// directory deployed by a Directory resource.
const selfPeerName = "self"

// prereqCRDs are the third-party CRDs a Directory needs. The operator never
// installs other operators: when these are missing it reports
// PrerequisitesMet=False and waits for cert-manager / CloudNativePG to be
// installed out-of-band.
var prereqCRDs = []string{
	"certificates.cert-manager.io",
	"issuers.cert-manager.io",
	"clusters.postgresql.cnpg.io",
}

// DirectoryReconciler deploys a self-contained OpenFSC directory peer per
// Directory resource: the prerequisite group CA + Manager group certificate +
// CloudNativePG cluster (server-side apply), the vendored OpenFSC umbrella
// (Helm), and the "self" Peer representing the directory. Readiness follows
// the Manager and Controller Deployments.
type DirectoryReconciler struct {
	Client client.Client // cached: Directory, Peer, Deployments
	Direct client.Client // uncached: CRD preflight + unstructured prerequisites
}

func (r *DirectoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&openfscv1.Directory{}).
		Named("directory").
		Complete(r); err != nil {
		return fmt.Errorf("setup directory controller: %w", err)
	}
	return nil
}

func (r *DirectoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var dir openfscv1.Directory
	if err := r.Client.Get(ctx, req.NamespacedName, &dir); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get Directory: %w", err)
	}

	if !dir.GetDeletionTimestamp().IsZero() {
		if controllerutil.ContainsFinalizer(&dir, directoryFinalizer) {
			if err := r.teardown(ctx, &dir); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&dir, directoryFinalizer)
			if err := r.Client.Update(ctx, &dir); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}
	if controllerutil.AddFinalizer(&dir, directoryFinalizer) {
		if err := r.Client.Update(ctx, &dir); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Preflight: the operator depends on cert-manager and CloudNativePG but
	// never installs them. Report clearly and wait while they are missing.
	missing, err := r.missingPrereqCRDs(ctx)
	if err != nil {
		return r.reflectDirectoryError(ctx, &dir, fmt.Errorf("check prerequisite CRDs: %w", err))
	}
	if len(missing) > 0 {
		msg := fmt.Sprintf("waiting for prerequisite CRDs: %s (install cert-manager and CloudNativePG)", strings.Join(missing, ", "))
		r.setCondition(&dir, openfscv1.ConditionPrerequisitesMet, metav1.ConditionFalse, "MissingCRDs", msg)
		return r.reflectDirectoryPending(ctx, &dir, msg)
	}
	r.setCondition(&dir, openfscv1.ConditionPrerequisitesMet, metav1.ConditionTrue, "Present", "cert-manager and CloudNativePG CRDs are present")

	// Provision (prerequisite resources + umbrella) when this generation has not
	// been deployed yet, or when the umbrella release has gone missing.
	if err := r.ensureDeployed(ctx, &dir); err != nil {
		return r.reflectDirectoryError(ctx, &dir, err)
	}

	// The operator owns the "self" Peer representing this directory.
	if err := r.ensureSelfPeer(ctx, &dir); err != nil {
		return r.reflectDirectoryError(ctx, &dir, fmt.Errorf("ensure self peer: %w", err))
	}

	ready, detail, err := directoryDeploymentsReady(ctx, r.Client, dir.Spec.Namespace)
	if err != nil {
		return r.reflectDirectoryError(ctx, &dir, err)
	}
	if !ready {
		return r.reflectDirectoryPending(ctx, &dir, detail)
	}

	dir.Status.Phase = openfscv1.PhaseActive
	dir.Status.Message = "OpenFSC Manager and Controller are running"
	dir.Status.ControllerURL = dir.Spec.ControllerURL
	dir.Status.ObservedGeneration = dir.Generation
	r.setCondition(&dir, openfscv1.ConditionReady, metav1.ConditionTrue, "Running", dir.Status.Message)
	if err := r.Client.Status().Update(ctx, &dir); err != nil {
		return ctrl.Result{}, fmt.Errorf("update Directory status: %w", err)
	}
	return ctrl.Result{RequeueAfter: resyncInterval}, nil
}

// ensureDeployed applies the directory prerequisites and installs/upgrades the
// OpenFSC umbrella. Skipped when the current generation is already deployed and
// the release still exists, so steady-state reconciles stay cheap.
func (r *DirectoryReconciler) ensureDeployed(ctx context.Context, dir *openfscv1.Directory) error {
	helmClient := helm.NewClient(dir.Spec.Namespace)

	deployed := meta.FindStatusCondition(dir.Status.Conditions, openfscv1.ConditionDeployed)
	if deployed != nil && deployed.Status == metav1.ConditionTrue && deployed.ObservedGeneration == dir.Generation {
		installed, err := helmClient.IsInstalled(umbrellaRelease)
		if err != nil {
			return fmt.Errorf("check umbrella release: %w", err)
		}
		if installed {
			return nil
		}
	}

	if err := ensureNamespace(ctx, r.Direct, dir.Spec.Namespace); err != nil {
		return err
	}
	if err := applyDirectoryResources(ctx, r.Direct, dir); err != nil {
		return fmt.Errorf("apply directory prerequisites: %w", err)
	}
	if err := r.installUmbrella(ctx, helmClient, dir); err != nil {
		return fmt.Errorf("install OpenFSC umbrella: %w", err)
	}

	r.setCondition(dir, openfscv1.ConditionDeployed, metav1.ConditionTrue, "Applied", "directory prerequisites and OpenFSC umbrella applied")
	return nil
}

// installUmbrella installs/upgrades the vendored OpenFSC umbrella as release
// "shared" with the embedded Fundament override and the Directory's settings.
func (r *DirectoryReconciler) installUmbrella(ctx context.Context, helmClient *helm.Client, dir *openfscv1.Directory) error {
	archive, err := charts.FS.ReadFile(charts.UmbrellaArchive)
	if err != nil {
		return fmt.Errorf("read umbrella chart: %w", err)
	}
	chrt, err := helm.LoadArchive(archive)
	if err != nil {
		return fmt.Errorf("load umbrella chart: %w", err)
	}

	override, err := charts.FS.ReadFile(charts.ValuesFundament)
	if err != nil {
		return fmt.Errorf("read umbrella override values: %w", err)
	}
	values, err := helm.ParseValues(override)
	if err != nil {
		return fmt.Errorf("parse umbrella override values: %w", err)
	}
	if err := helm.ApplySet(values, map[string]string{
		"fullnameOverride":                        umbrellaRelease,
		"global.groupID":                          dir.Spec.GroupID,
		"open-fsc-manager.config.groupID":         dir.Spec.GroupID,
		"open-fsc-manager.config.directoryPeerID": dir.Spec.PeerID,
	}); err != nil {
		return fmt.Errorf("apply umbrella overrides: %w", err)
	}
	setNestedValue(values, stringsToAny(dir.Spec.AutoSignGrants), "open-fsc-manager", "config", "autoSignGrants")

	if err := helmClient.UpgradeInstall(ctx, umbrellaRelease, chrt, values); err != nil {
		return fmt.Errorf("upgrade-install umbrella: %w", err)
	}
	return nil
}

// ensureSelfPeer creates (or updates the spec of) the "self" Peer representing
// this Directory, so the install is immediately visible as a group member.
func (r *DirectoryReconciler) ensureSelfPeer(ctx context.Context, dir *openfscv1.Directory) error {
	spec := openfscv1.PeerSpec{
		GroupID:        dir.Spec.GroupID,
		PeerID:         dir.Spec.PeerID,
		ManagerAddress: fmt.Sprintf("https://%s.%s:8443", managerExternalName, dir.Spec.Namespace),
		Directory:      true,
	}

	var peer openfscv1.Peer
	err := r.Client.Get(ctx, types.NamespacedName{Name: selfPeerName}, &peer)
	if apierrors.IsNotFound(err) {
		peer = openfscv1.Peer{
			ObjectMeta: metav1.ObjectMeta{Name: selfPeerName},
			Spec:       spec,
		}
		if err := r.Client.Create(ctx, &peer); err != nil {
			return fmt.Errorf("create self peer: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get self peer: %w", err)
	}
	if peer.Spec == spec {
		return nil
	}
	peer.Spec = spec
	if err := r.Client.Update(ctx, &peer); err != nil {
		return fmt.Errorf("update self peer: %w", err)
	}
	return nil
}

// teardown removes everything the Directory provisioned: the self Peer, the
// umbrella release and the prerequisite resources. The namespace is left in
// place (gateways may still be tearing down in it).
func (r *DirectoryReconciler) teardown(ctx context.Context, dir *openfscv1.Directory) error {
	peer := &openfscv1.Peer{ObjectMeta: metav1.ObjectMeta{Name: selfPeerName}}
	if err := r.Client.Delete(ctx, peer); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete self peer: %w", err)
	}
	if err := helm.NewClient(dir.Spec.Namespace).Uninstall(umbrellaRelease); err != nil {
		return fmt.Errorf("uninstall umbrella: %w", err)
	}
	return deleteDirectoryResources(ctx, r.Direct, dir)
}

// missingPrereqCRDs returns the prerequisite CRDs not present in the cluster.
func (r *DirectoryReconciler) missingPrereqCRDs(ctx context.Context) ([]string, error) {
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

func (r *DirectoryReconciler) setCondition(dir *openfscv1.Directory, condType string, status metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(&dir.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: dir.Generation,
		Reason:             reason,
		Message:            msg,
	})
}

// reflectDirectoryPending records a not-yet-ready state and requeues shortly.
func (r *DirectoryReconciler) reflectDirectoryPending(ctx context.Context, dir *openfscv1.Directory, msg string) (ctrl.Result, error) {
	dir.Status.Phase = openfscv1.PhasePending
	dir.Status.Message = msg
	dir.Status.ObservedGeneration = dir.Generation
	r.setCondition(dir, openfscv1.ConditionReady, metav1.ConditionFalse, "Pending", msg)
	if err := r.Client.Status().Update(ctx, dir); err != nil {
		return ctrl.Result{}, fmt.Errorf("update Directory status: %w", err)
	}
	return ctrl.Result{RequeueAfter: pendingRetryInterval}, nil
}

// reflectDirectoryError records a reconcile error and returns it (backoff).
func (r *DirectoryReconciler) reflectDirectoryError(ctx context.Context, dir *openfscv1.Directory, reconcileErr error) (ctrl.Result, error) {
	dir.Status.Phase = openfscv1.PhaseError
	dir.Status.Message = reconcileErr.Error()
	dir.Status.ObservedGeneration = dir.Generation
	r.setCondition(dir, openfscv1.ConditionReady, metav1.ConditionFalse, "ReconcileError", reconcileErr.Error())
	_ = r.Client.Status().Update(ctx, dir)
	return ctrl.Result{}, reconcileErr
}

// directoryDeploymentsReady reports whether both the Manager and Controller
// Deployments are Available, with a human-readable detail when they are not.
func directoryDeploymentsReady(ctx context.Context, c client.Client, ns string) (bool, string, error) {
	for _, name := range []string{managerDeployment, controllerDeployment} {
		var deploy appsv1.Deployment
		err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &deploy)
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

// getDirectory returns the Directory the cluster's peers and gateways belong
// to. With several Directory resources (unsupported but possible) the first by
// name wins, deterministically. Returns nil when none exist.
func getDirectory(ctx context.Context, c client.Client) (*openfscv1.Directory, error) {
	var list openfscv1.DirectoryList
	if err := c.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list Directories: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Name < list.Items[j].Name })
	return &list.Items[0], nil
}

// setNestedValue sets a value at the given path in a Helm values map, creating
// intermediate maps as needed (used for list values, which --set syntax does
// not express cleanly).
func setNestedValue(values map[string]any, value any, path ...string) {
	node := values
	for _, key := range path[:len(path)-1] {
		child, ok := node[key].(map[string]any)
		if !ok {
			child = map[string]any{}
			node[key] = child
		}
		node = child
	}
	node[path[len(path)-1]] = value
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
