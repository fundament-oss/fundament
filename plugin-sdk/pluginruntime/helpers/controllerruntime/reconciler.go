// Package controllerruntime provides optional scaffolding for plugins that use
// controller-runtime to manage Kubernetes resources via reconcilers.
//
// Plugin authors should import this package and use SetupManager to configure
// a controller-runtime manager with scheme registration.
package controllerruntime

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var setLoggerOnce sync.Once

// setDefaultLogger routes controller-runtime's logging into the plugin's slog
// output.
//
// Without this, controller-runtime discards every log line a reconciler emits
// and prints a stack trace complaining that SetLogger was never called.
// Anything a plugin reports through log.FromContext -- the usual way a
// reconciler explains why it could not make progress -- then vanishes, which is
// the worst possible failure mode for a background loop nobody is watching.
//
// It is set here rather than in each plugin's Start because the logger is
// process-global and every plugin that builds a manager needs it. sync.Once
// keeps a second SetupManager call from re-registering it.
func setDefaultLogger() {
	setLoggerOnce.Do(func() {
		ctrl.SetLogger(logr.FromSlogHandler(slog.Default().Handler()))
	})
}

// SetupManager creates a controller-runtime manager with the given scheme.
func SetupManager(scheme *runtime.Scheme, opts *ctrl.Options) (manager.Manager, error) {
	setDefaultLogger()
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
