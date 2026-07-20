// Package app wires up the cluster-worker application.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	clusterhandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
	namespacehandler "github.com/fundament-oss/fundament/cluster-worker/pkg/handler/namespace"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/usersync"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/outbox"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/reconcile"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/status"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Config holds all configuration for the cluster-worker application.
type Config struct {
	HealthPort      int           `env:"HEALTH_PORT" envDefault:"8097"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`

	Gardener GardenerConfig `envPrefix:"GARDENER_"`

	Outbox    outbox.Config         `envPrefix:"OUTBOX_"`
	Status    status.Config         `envPrefix:"STATUS_"`
	Reconcile reconcile.Config      `envPrefix:"RECONCILE_"`
	Cluster   clusterhandler.Config `envPrefix:"CLUSTER_"`
}

// GardenerConfig configures the Gardener client and the provider defaults the
// cluster-worker stamps onto Shoots. Every field is read under the GARDENER_ env
// prefix (factored out here rather than repeated per tag), so each env var is the
// UPPER_SNAKE of the field name, e.g. ProviderType -> GARDENER_PROVIDER_TYPE.
// The zero values target Gardener's local provider; non-local providers such as
// metal override the provider fields (see the deployment's Helm values).
type GardenerConfig struct {
	Mode       string `env:"MODE"`       // mock or real
	Kubeconfig string `env:"KUBECONFIG"` // required when Mode == real

	ProviderType           string `env:"PROVIDER_TYPE"`            // e.g. local, metal
	CloudProfile           string `env:"CLOUD_PROFILE"`            // e.g. local, metal
	Region                 string `env:"REGION"`                   // default Shoot region when the cluster specifies none
	CredentialsBindingName string `env:"CREDENTIALS_BINDING_NAME"` // e.g. local, metal-credentials

	// Shared credentials that the per-project CredentialsBindings reference. The
	// in-code defaults target the local provider's WorkloadIdentity
	// (garden-local/local); metal uses a Secret and must set all three.
	CredentialsRef           string `env:"CREDENTIALS_REF"`             // "namespace/name"
	CredentialsRefKind       string `env:"CREDENTIALS_REF_KIND"`        // Secret or WorkloadIdentity
	CredentialsRefAPIVersion string `env:"CREDENTIALS_REF_API_VERSION"` // v1 or security.gardener.cloud/v1alpha1

	MachineImageName    string `env:"MACHINE_IMAGE_NAME"`
	MachineImageVersion string `env:"MACHINE_IMAGE_VERSION"`
	DefaultMachineType  string `env:"DEFAULT_MACHINE_TYPE"`

	NodesCIDR    string `env:"NODES_CIDR"` // empty => provider IPAM allocates (metal); local falls back to 10.0.0.0/16
	PodsCIDR     string `env:"PODS_CIDR"`
	ServicesCIDR string `env:"SERVICES_CIDR"`

	// Provider extension configs (raw JSON) stamped verbatim onto Shoots, plus
	// extra Shoot annotations (a JSON object). Used by non-local providers (metal).
	InfrastructureConfig string `env:"INFRASTRUCTURE_CONFIG"`
	ControlPlaneConfig   string `env:"CONTROL_PLANE_CONFIG"`
	ShootAnnotations     string `env:"SHOOT_ANNOTATIONS"`
}

// ReadyChecker reports whether a worker is ready to serve traffic.
type ReadyChecker interface {
	IsReady() bool
}

// App holds the wired-up application components.
type App struct {
	pool            *pgxpool.Pool
	registry        *handler.Registry
	outboxWorker    *outbox.Worker
	statusWorker    *status.Worker
	reconcileWorker *reconcile.Worker
	healthServer    *http.Server
	logger          *slog.Logger
	cfg             *Config
}

// New creates and wires up the cluster-worker application.
func New(pool *pgxpool.Pool, logger *slog.Logger, cfg *Config) (*App, error) {
	gardenerClient, err := createGardenerClient(cfg, logger)
	if err != nil {
		return nil, err
	}

	registry := handler.NewRegistry()

	// Cluster handler (sync, status, reconcile)
	ch := clusterhandler.New(pool, gardenerClient, gardenerClient, logger, cfg.Cluster)
	registry.RegisterSync(handler.EntityCluster, ch)
	registry.RegisterSync(handler.EntityNodePool, ch)
	registry.RegisterStatus(ch)
	registry.RegisterReconcile(ch)

	// User sync handler (SA/CRB lifecycle on shoots)
	shootAccess, err := createShootAccess(cfg.Gardener.Mode, gardenerClient, logger)
	if err != nil {
		return nil, err
	}
	ush := usersync.New(pool, shootAccess, logger)
	registry.RegisterSync(handler.EntityOrgUser, ush)
	registry.RegisterSync(handler.EntityProjectMember, ush)
	registry.RegisterSyncForEvent(handler.EntityCluster, dbconst.ClusterOutboxEvent_Ready, ush)
	registry.RegisterReconcile(ush)

	// Namespace sync handler (v1/Namespace lifecycle on shoots). Shares the same
	// ShootAccess instance as usersync and the outbox worker's MaxRetries so its
	// reconcile enqueue uses the same exhaustion threshold. Like usersync, it
	// also subscribes to the cluster-ready event to fan out a sync for every
	// active namespace on the cluster.
	nsh := namespacehandler.New(pool, shootAccess, cfg.Outbox.MaxRetries, logger)
	registry.RegisterSync(handler.EntityNamespace, nsh)
	registry.RegisterSyncForEvent(handler.EntityCluster, dbconst.ClusterOutboxEvent_Ready, nsh)
	registry.RegisterReconcile(nsh)

	// Workers
	outboxWorker := outbox.New(pool, registry, logger, cfg.Outbox)
	statusWorker := status.New(registry, logger, cfg.Status)
	reconcileWorker := reconcile.New(registry, logger, cfg.Reconcile)

	// Health server
	healthServer := startHealthServer(cfg.HealthPort, logger, outboxWorker, statusWorker, reconcileWorker)

	return &App{
		pool:            pool,
		registry:        registry,
		outboxWorker:    outboxWorker,
		statusWorker:    statusWorker,
		reconcileWorker: reconcileWorker,
		healthServer:    healthServer,
		logger:          logger,
		cfg:             cfg,
	}, nil
}

// Run starts all workers concurrently and blocks until shutdown.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("cluster-worker starting",
		"poll_interval", a.cfg.Outbox.PollInterval,
		"reconcile_interval", a.cfg.Reconcile.Interval,
		"status_interval", a.cfg.Status.Interval,
		"max_retries", a.cfg.Outbox.MaxRetries,
		"gardener_mode", a.cfg.Gardener.Mode)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return a.outboxWorker.Run(ctx) })
	g.Go(func() error { return a.statusWorker.Run(ctx) })
	g.Go(func() error { return a.reconcileWorker.Run(ctx) })

	err := g.Wait()

	// Graceful shutdown
	a.logger.Info("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()
	if shutdownErr := a.healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
		a.logger.Error("health server shutdown error", "error", shutdownErr)
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("worker error: %w", err)
	}

	a.logger.Info("cluster-worker stopped")
	return nil
}

func createGardenerClient(cfg *Config, logger *slog.Logger) (gardener.Client, error) {
	g := cfg.Gardener
	switch g.Mode {
	case "mock":
		logger.Info("using mock Gardener client (in-memory)")
		return gardener.NewMock(logger), nil

	case "real":
		if g.Kubeconfig == "" {
			return nil, fmt.Errorf("GARDENER_KUBECONFIG required for real mode")
		}
		providerCfg := gardener.NewProviderConfig()
		if g.ProviderType != "" {
			providerCfg.Type = g.ProviderType
		}
		if g.CloudProfile != "" {
			providerCfg.CloudProfile = g.CloudProfile
		}
		if g.Region != "" {
			providerCfg.Region = g.Region
		}
		if g.CredentialsBindingName != "" {
			providerCfg.CredentialsBindingName = g.CredentialsBindingName
		}
		if g.CredentialsRef != "" {
			providerCfg.CredentialsRef = g.CredentialsRef
		}
		if g.CredentialsRefKind != "" {
			providerCfg.CredentialsRefKind = g.CredentialsRefKind
		}
		if g.CredentialsRefAPIVersion != "" {
			providerCfg.CredentialsRefAPIVersion = g.CredentialsRefAPIVersion
		}
		if g.MachineImageName != "" {
			providerCfg.MachineImageName = g.MachineImageName
		}
		if g.MachineImageVersion != "" {
			providerCfg.MachineImageVersion = g.MachineImageVersion
		}
		if g.DefaultMachineType != "" {
			providerCfg.DefaultMachineType = g.DefaultMachineType
		}
		if g.NodesCIDR != "" {
			providerCfg.NodesCIDR = g.NodesCIDR
		}
		if g.PodsCIDR != "" {
			providerCfg.PodsCIDR = g.PodsCIDR
		}
		if g.ServicesCIDR != "" {
			providerCfg.ServicesCIDR = g.ServicesCIDR
		}
		if g.InfrastructureConfig != "" {
			providerCfg.InfrastructureConfig = g.InfrastructureConfig
		}
		if g.ControlPlaneConfig != "" {
			providerCfg.ControlPlaneConfig = g.ControlPlaneConfig
		}
		if g.ShootAnnotations != "" {
			anns := map[string]string{}
			if err := json.Unmarshal([]byte(g.ShootAnnotations), &anns); err != nil {
				return nil, fmt.Errorf("parse GARDENER_SHOOT_ANNOTATIONS: %w", err)
			}
			providerCfg.ShootAnnotations = anns
		}

		logger.Info("using real Gardener client",
			"kubeconfig", g.Kubeconfig,
			"provider", providerCfg.Type,
			"cloudProfile", providerCfg.CloudProfile)
		client, err := gardener.NewReal(g.Kubeconfig, providerCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("create gardener client: %w", err)
		}
		return client, nil

	default:
		return nil, fmt.Errorf("invalid GARDENER_MODE: %s (must be mock or real)", g.Mode)
	}
}

func createShootAccess(gardenerMode string, gardenerClient gardener.Client, logger *slog.Logger) (shoot.ShootAccess, error) {
	switch gardenerMode {
	case "mock":
		logger.Info("using mock shoot access (in-memory)")
		return shoot.NewMockShootAccess(logger), nil
	case "real":
		logger.Info("using real shoot access (AdminKubeconfigRequest)")
		return shoot.NewRealShootAccess(gardenerClient, logger), nil
	default:
		return nil, fmt.Errorf("invalid GARDENER_MODE: %s (must be mock or real)", gardenerMode)
	}
}

func startHealthServer(port int, logger *slog.Logger, checkers ...ReadyChecker) *http.Server {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/livez", func(resp http.ResponseWriter, _ *http.Request) {
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write([]byte("ok"))
	})
	healthMux.HandleFunc("/readyz", func(resp http.ResponseWriter, _ *http.Request) {
		for _, c := range checkers {
			if !c.IsReady() {
				resp.WriteHeader(http.StatusServiceUnavailable)
				_, _ = resp.Write([]byte("not ready"))
				return
			}
		}
		resp.WriteHeader(http.StatusOK)
		_, _ = resp.Write([]byte("ready"))
	})

	healthServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           healthMux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("health server starting", "port", port)
		if err := healthServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server error", "error", err)
		}
	}()

	return healthServer
}
