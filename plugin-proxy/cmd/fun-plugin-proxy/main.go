package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/gardener"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/assets"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/config"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/installproxy"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/kube"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting plugin-proxy",
		"listen_addr", cfg.ListenAddr,
		"internal_listen_addr", cfg.InternalListenAddr,
		"log_level", cfg.LogLevel.String(),
		"mode", cfg.Mode,
	)

	publicMux := http.NewServeMux()
	registerHealth(publicMux)

	internalMux := http.NewServeMux()
	registerHealth(internalMux)

	// Static assets + strict CSP.
	cfgCsp := &assets.CSPConfig{
		ConnectSrc:     []string{cfg.KubeAPIProxyOrigin, cfg.PluginProxyOrigin},
		FormAction:     []string{cfg.KubeAPIProxyOrigin, cfg.PluginProxyOrigin},
		FrameAncestors: []string{cfg.ConsoleOrigin},
	}

	var (
		resolver      assets.ClusterResolver
		fetcher       assets.Fetcher
		clusterAuthz  installproxy.ClusterAuthorizer
		backend       installproxy.Backend
		clusterAccess service.ClusterAccess
	)
	switch cfg.Mode {
	case "real":
		gardenerClient, err := gardener.New(cfg.GardenerKubeconfig, logger)
		if err != nil {
			return fmt.Errorf("create gardener client: %w", err)
		}
		adminCache := gardener.NewAdminKubeconfigCache(gardenerClient, logger)
		admin := kube.NewAdminKubeconfigCache(adminCache)

		authzClient, err := authz.New(cfg.OpenFGA)
		if err != nil {
			return fmt.Errorf("create OpenFGA client: %w", err)
		}

		clusterAccess, err = service.NewGardenerClusterAccess(adminCache)
		if err != nil {
			return fmt.Errorf("create cluster access: %w", err)
		}

		resolver = assets.Resolver{}
		fetcher = &assets.PodFetcher{AdminKubeconfig: admin}
		clusterAuthz = installproxy.Authz{Client: authzClient}
		backend = &installproxy.ClusterProxyBackend{AdminKubeconfig: admin}
	case "mock":
		resolver = assets.MockResolver{ClusterID: service.MockClusterID}
		fetcher = assets.MockFetcher{Logger: logger}
		clusterAuthz = installproxy.MockAuthz{}
		backend = installproxy.MockBackend{Logger: logger}
		clusterAccess = service.NewMockClusterAccess()
	default:
		// config.FromEnv already validates this; guard against future drift.
		panic(fmt.Sprintf("unhandled plugin-proxy mode %q", cfg.Mode))
	}

	// SDK route stub — Plan E supplies the bundle. Register before /plugins/
	// for readability; ServeMux's longest-match routing already does the right
	// thing regardless of order.
	publicMux.HandleFunc("/plugins/sdk/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "sdk bundle not built (Plan E)", http.StatusNotFound)
	})
	publicMux.Handle("/plugins/", assets.NewHandler(resolver, fetcher, cfgCsp, logger))

	// Installation proxy (cross-site → wrap in CORS). The PluginToken rides
	// in Authorization, so credentials mode stays off — do not enable
	// AllowCredentials.
	installHandler := installproxy.New([]byte(cfg.JWTSecret), clusterAuthz, backend, logger)
	installCORS := cors.New(cors.Options{
		AllowedOrigins: []string{cfg.PluginProxyOrigin},
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	})
	publicMux.Handle("/installations/", installCORS.Handler(installHandler))

	s := service.New(logger, clusterAccess)

	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)

	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		validate.NewInterceptor(),
		loggingInterceptor,
	)

	internalMux.Handle(pluginproxyv1connect.NewPluginInstallationServiceHandler(s, interceptors))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	publicSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           publicMux,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
	internalSrv := &http.Server{
		Addr:              cfg.InternalListenAddr,
		Handler:           internalMux,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 2)
	go func() {
		logger.Info("public surface listening", "addr", cfg.ListenAddr)
		if err := publicSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("public server: %w", err)
		}
	}()
	go func() {
		logger.Info("internal surface listening", "addr", cfg.InternalListenAddr)
		if err := internalSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("internal server: %w", err)
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-serveErr:
		logger.Error("server error, shutting down", "error", err)
		runErr = err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := publicSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("public server shutdown", "error", err)
	}
	if err := internalSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("internal server shutdown", "error", err)
	}
	return runErr
}

func registerHealth(mux *http.ServeMux) {
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
