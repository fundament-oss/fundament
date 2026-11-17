// gardener-extension-tenant-metrics runs on a Gardener seed. For every shoot
// it adds a scrape job to the shoot's Prometheus that collects container
// metrics from tenant namespaces; Gardener itself collects kube-system only.
package main

import (
	"log/slog"
	"os"

	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/fundament-oss/fundament/gardener-extension-tenant-metrics/pkg/controller"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctrl.SetLogger(logr.FromSlogHandler(logger.Handler()))

	err := run()
	if err != nil {
		logger.Error("gardener-extension-tenant-metrics stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(extensionsv1alpha1.AddToScheme(scheme))

	// Watch only Gardener's cAdvisor jobs, not every ScrapeConfig on the seed.
	// Unstructured reads bypass the cache, so the tenant job is read live.
	scrapeConfigs := &unstructured.Unstructured{}
	scrapeConfigs.SetGroupVersionKind(schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1alpha1", Kind: "ScrapeConfig"})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{ByObject: map[client.Object]cache.ByObject{
			scrapeConfigs: {Field: fields.OneTermEqualSelector("metadata.name", controller.SourceName)},
		}},
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: ":8081",
		LeaderElection:         true,
		LeaderElectionID:       "gardener-extension-tenant-metrics",
	})
	if err != nil {
		return err //nolint:wrapcheck // the caller logs it
	}

	err = (&controller.Reconciler{Client: mgr.GetClient()}).SetupWithManager(mgr)
	if err != nil {
		return err //nolint:wrapcheck // already wrapped
	}
	err = mgr.AddHealthzCheck("healthz", healthz.Ping)
	if err != nil {
		return err //nolint:wrapcheck // the caller logs it
	}

	return mgr.Start(ctrl.SetupSignalHandler()) //nolint:wrapcheck // the caller logs it
}
