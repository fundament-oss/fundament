// The openfsc-operator turns FSCInstallation resources (openfsc.fundament.io)
// into running OpenFSC peers: one installation per team namespace, holding the
// OpenFSC core (Manager, Controller, audit/transaction logs on a CloudNativePG
// cluster) and one gateway workload per declared inway/outway.
//
// The operator depends on cert-manager and CloudNativePG but never installs
// them; an FSCInstallation reports PrerequisitesMet=False until their CRDs
// exist.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
	"github.com/fundament-oss/fundament/openfsc-operator/internal/controller"
)

type config struct {
	// MetricsPort serves Prometheus metrics; 0 disables the endpoint.
	MetricsPort int `env:"METRICS_PORT" envDefault:"8080"`
	// HealthPort serves the /livez and /readyz probes.
	HealthPort int `env:"HEALTH_PORT" envDefault:"8081"`
	// LeaderElect enables leader election so multiple replicas can run.
	LeaderElect bool `env:"LEADER_ELECT" envDefault:"true"`
	// LogLevel is the slog level (debug, info, warn, error).
	LogLevel slog.Level `env:"LOG_LEVEL" envDefault:"info"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var cfg config
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("parse env: %w", err)
	}

	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})
	logger := slog.New(slogHandler)
	slog.SetDefault(logger)
	crlog.SetLogger(logr.FromSlogHandler(slogHandler))

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apiextensionsv1.AddToScheme,
		openfscv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			return fmt.Errorf("build scheme: %w", err)
		}
	}

	metricsAddr := "0"
	if cfg.MetricsPort != 0 {
		metricsAddr = fmt.Sprintf(":%d", cfg.MetricsPort)
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                        scheme,
		Metrics:                       metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress:        fmt.Sprintf(":%d", cfg.HealthPort),
		LivenessEndpointName:          "/livez",
		ReadinessEndpointName:         "/readyz",
		LeaderElection:                cfg.LeaderElect,
		LeaderElectionID:              "openfsc-operator.fundament.io",
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("cache-sync", func(req *http.Request) error {
		ctx, cancel := context.WithTimeout(req.Context(), 100*time.Millisecond)
		defer cancel()
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return fmt.Errorf("cache not synced")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("add readyz check: %w", err)
	}

	// Direct (uncached) client for resources the manager should not build
	// informers for: the CRD preflight, cert-manager Certificates and the mTLS
	// Secrets behind the Administration API clients.
	direct, err := client.New(mgr.GetConfig(), client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("create direct client: %w", err)
	}

	reconciler := &controller.FSCInstallationReconciler{
		Client: mgr.GetClient(),
		Direct: direct,
		Admin:  controller.NewAdminClients(direct),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup reconciler: %w", err)
	}

	logger.Info("openfsc-operator starting", "leaderElect", cfg.LeaderElect)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("start manager: %w", err)
	}
	return nil
}
