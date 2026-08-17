// Package crd provides optional helpers for verifying that CRDs declared by a
// plugin actually exist in the target Kubernetes cluster.
package crd

import (
	"context"
	"fmt"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Default polling schedule for WaitEstablished.
const (
	establishedPollInterval = 2 * time.Second
	establishedPollTimeout  = 60 * time.Second
)

// Exists checks whether a CRD with the given name exists in the cluster.
// The name should be in the form "<plural>.<group>", e.g. "certificates.cert-manager.io".
func Exists(ctx context.Context, c client.Client, name string) (bool, error) {
	var crd apiextensionsv1.CustomResourceDefinition
	err := c.Get(ctx, types.NamespacedName{Name: name}, &crd)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking CRD %q: %w", name, err)
	}
	return true, nil
}

// VerifyAll checks that all named CRDs exist in the cluster, returning an error
// listing any that are missing.
func VerifyAll(ctx context.Context, c client.Client, names []string) error {
	var missing []string
	for _, name := range names {
		ok, err := Exists(ctx, c, name)
		if err != nil {
			return err
		}
		if !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing CRDs: %v", missing)
	}
	return nil
}

// WaitEstablished polls until every named CRD reports Established=True. Until it
// does, the API server has no REST mapping for the new kinds and both Get and
// Create fail. A CRD that is not visible yet — NotFound, or a no-match from a
// stale RESTMapper — means keep polling; that is normal right after an apply.
func WaitEstablished(ctx context.Context, c client.Client, names []string) error {
	err := wait.PollUntilContextTimeout(ctx, establishedPollInterval, establishedPollTimeout, true,
		func(ctx context.Context) (bool, error) {
			for _, name := range names {
				var crd apiextensionsv1.CustomResourceDefinition
				if err := c.Get(ctx, types.NamespacedName{Name: name}, &crd); err != nil {
					if apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
						return false, nil // not yet visible; keep polling
					}
					return false, err
				}
				if !isEstablished(&crd) {
					return false, nil
				}
			}
			return true, nil
		})
	if err != nil {
		return fmt.Errorf("wait for established CRDs %v: %w", names, err)
	}
	return nil
}

func isEstablished(crd *apiextensionsv1.CustomResourceDefinition) bool {
	for _, cond := range crd.Status.Conditions {
		if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
			return true
		}
	}
	return false
}
