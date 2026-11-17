package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// operationAnnotation is how gardenlet asks an extension to act; the
	// extension removes it once it has picked the request up.
	operationAnnotation = "gardener.cloud/operation"
	operationMigrate    = "migrate"
	operationRestore    = "restore"

	// sourceRetry is how soon a shoot without Gardener's cAdvisor job is looked at again.
	sourceRetry = 30 * time.Second
	// resync rewrites the tenant job now and then, which also repairs an
	// update to Gardener's job that was missed.
	resync = time.Hour
)

// SourceName is Gardener's cAdvisor job, the only ScrapeConfig this
// controller needs to watch.
const SourceName = sourceName

// Reconciler keeps the tenant scrape job next to each shoot's Prometheus, in
// the shoot's control-plane namespace on the seed.
type Reconciler struct {
	client.Client
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	source := &unstructured.Unstructured{}
	source.SetGroupVersionKind(scrapeConfigGVK)

	err := ctrl.NewControllerManagedBy(mgr).
		Named("tenant-metrics").
		For(&extensionsv1alpha1.Extension{}, builderPredicates()).
		// Gardener rewrites its cAdvisor job on an upgrade; the copy follows.
		Watches(source, handler.EnqueueRequestsFromMapFunc(r.extensionsFor)).
		Complete(r)
	if err != nil {
		return fmt.Errorf("set up controller: %w", err)
	}
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ext := &extensionsv1alpha1.Extension{}
	err := r.Get(ctx, req.NamespacedName, ext)
	if apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get extension: %w", err)
	}
	// A deleted Extension takes the scrape job with it through its owner reference.
	if ext.Spec.Type != ExtensionType || !ext.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	operation := operationType(ext)
	description := "tenant scrape job is in place"
	result := ctrl.Result{RequeueAfter: resync}

	// A control plane that migrates away leaves this seed with its namespace.
	// Neither a missing nor an unrecognised Gardener job fails the Extension:
	// gardenlet would hold up every shoot reconcile for a metrics extra.
	if operation != gardencorev1beta1.LastOperationTypeMigrate {
		var found bool
		found, err = r.apply(ctx, ext)
		switch {
		case errors.Is(err, errUnknownShape):
			ctrl.LoggerFrom(ctx).Error(err, "tenant scrape job not written")
			description = "tenant scrape job not written: " + err.Error()
			err = nil
		case err == nil && !found:
			// Gardener creates its job later in the same shoot reconcile.
			description = "waiting for Gardener's " + sourceName + " scrape job"
			result.RequeueAfter = sourceRetry
		}
	}

	statusErr := r.report(ctx, ext, operation, description, err)
	if err != nil {
		return ctrl.Result{}, err
	}
	return result, statusErr
}

// apply writes the tenant scrape job; found is false while Gardener's own job
// does not exist yet.
func (r *Reconciler) apply(ctx context.Context, ext *extensionsv1alpha1.Extension) (bool, error) {
	source := &unstructured.Unstructured{}
	source.SetGroupVersionKind(scrapeConfigGVK)
	err := r.Get(ctx, types.NamespacedName{Namespace: ext.Namespace, Name: sourceName}, source)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get %s: %w", sourceName, err)
	}

	sourceSpec, _, err := unstructured.NestedMap(source.Object, "spec")
	if err != nil {
		return false, fmt.Errorf("read %s: %w", sourceName, err)
	}
	spec, err := tenantSpec(sourceSpec)
	if err != nil {
		return false, err
	}

	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(scrapeConfigGVK)
	target.SetNamespace(ext.Namespace)
	target.SetName(targetName)

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, target, func() error {
		// The shoot's Prometheus selects its scrape jobs by this label.
		target.SetLabels(map[string]string{"prometheus": "shoot"})
		target.Object["spec"] = spec
		return controllerutil.SetControllerReference(ext, target, r.Scheme())
	})
	if err != nil {
		return false, fmt.Errorf("write %s: %w", targetName, err)
	}
	return true, nil
}

// report takes gardenlet's request off the Extension and records the outcome;
// gardenlet waits for both.
func (r *Reconciler) report(
	ctx context.Context,
	ext *extensionsv1alpha1.Extension,
	operation gardencorev1beta1.LastOperationType,
	description string,
	failure error,
) error {
	if _, requested := ext.Annotations[operationAnnotation]; requested {
		before := ext.DeepCopy()
		delete(ext.Annotations, operationAnnotation)
		err := r.Patch(ctx, ext, client.MergeFrom(before))
		if err != nil {
			return fmt.Errorf("remove operation annotation: %w", err)
		}
	}

	before := ext.DeepCopy()
	now := metav1.Now()
	last := &gardencorev1beta1.LastOperation{
		Type:           operation,
		State:          gardencorev1beta1.LastOperationStateSucceeded,
		Progress:       100,
		Description:    description,
		LastUpdateTime: now,
	}
	ext.Status.LastError = nil
	if failure != nil {
		last.State = gardencorev1beta1.LastOperationStateError
		last.Progress = 0
		last.Description = failure.Error()
		ext.Status.LastError = &gardencorev1beta1.LastError{Description: failure.Error(), LastUpdateTime: &now}
	}
	ext.Status.LastOperation = last
	ext.Status.ObservedGeneration = ext.Generation

	err := r.Status().Patch(ctx, ext, client.MergeFrom(before))
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

// operationType names what gardenlet asked for.
func operationType(ext *extensionsv1alpha1.Extension) gardencorev1beta1.LastOperationType {
	switch ext.Annotations[operationAnnotation] {
	case operationMigrate:
		return gardencorev1beta1.LastOperationTypeMigrate
	case operationRestore:
		return gardencorev1beta1.LastOperationTypeRestore
	}
	if ext.Status.LastOperation == nil {
		return gardencorev1beta1.LastOperationTypeCreate
	}
	return gardencorev1beta1.LastOperationTypeReconcile
}

// builderPredicates lets spec changes and gardenlet's requests through, and
// keeps this controller's own status writes from starting a new round.
func builderPredicates() builder.ForOption {
	return builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}))
}

// extensionsFor maps a change to Gardener's cAdvisor job to the Extension in
// the same namespace.
func (r *Reconciler) extensionsFor(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetName() != sourceName {
		return nil
	}
	list := &extensionsv1alpha1.ExtensionList{}
	err := r.List(ctx, list, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		// The hourly resync writes the job again.
		ctrl.LoggerFrom(ctx).Error(err, "list extensions", "namespace", obj.GetNamespace())
		return nil
	}
	var requests []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.Type == ExtensionType {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
	}
	return requests
}
