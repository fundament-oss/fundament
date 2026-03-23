// Package controllerruntime provides optional scaffolding for plugins that use
// controller-runtime to manage Kubernetes resources via reconcilers.
//
// Plugin authors should import this package and use SetupManager to configure
// a controller-runtime manager with scheme registration.
package controllerruntime

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// SetupManager creates a controller-runtime manager with the given scheme.
func SetupManager(scheme *runtime.Scheme, opts *ctrl.Options) (manager.Manager, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get kubeconfig: %w", err)
	}
	opts.Scheme = scheme
	mgr, err := ctrl.NewManager(cfg, *opts)
	if err != nil {
		return nil, fmt.Errorf("unable to create controller manager: %w", err)
	}
	return mgr, nil
}
