// Package crd provides optional helpers for verifying that CRDs declared by a
// plugin actually exist in the target Kubernetes cluster.
package crd

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
