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

	"github.com/fundament-oss/fundament/common/auth"
	openfgaauthz "github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/assets"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/config"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/installproxy"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/kube"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/service"
	"github.com/google/uuid"
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
		fetcher assets.Fetcher
		authz   installproxy.ClusterAuthorizer
		backend installproxy.Backend
	)
	switch cfg.Mode {
	case "real":
		admin := kube.NewAdminKubeconfigCache()
		fetcher = &assets.PodFetcher{AdminKubeconfig: admin}
		authz = installproxy.Authz{}
		backend = &installproxy.ClusterProxyBackend{AdminKubeconfig: admin}
	case "mock":
		authz = installproxy.MockAuthz{}
		backend = installproxy.MockBackend{Logger: logger}

		// If a plugin sandbox kubeconfig is provided, fetch real assets from
		// the sandbox cluster instead of serving MockFetcher's stub HTML.
		// The clusterID in the request URL is used verbatim; the local
		// AdminKubeconfigCache returns the same sandbox kubeconfig regardless.
		if cfg.PluginSandboxKubeconfig != "" {
			admin, err := kube.NewAdminKubeconfigCacheFromFile(cfg.PluginSandboxKubeconfig)
			if err != nil {
				return fmt.Errorf("load plugin sandbox kubeconfig: %w", err)
			}
			fetcher = &assets.PodFetcher{AdminKubeconfig: admin}
			logger.Info("mock mode: fetching plugin assets from sandbox cluster",
				"kubeconfig", cfg.PluginSandboxKubeconfig)
		} else {
			fetcher = assets.MockFetcher{Logger: logger}
		}
	default:
		// config.FromEnv already validates this; guard against future drift.
		panic(fmt.Sprintf("unhandled plugin-proxy mode %q", cfg.Mode))
	}

	// SDK bundle at /plugins/sdk/v1/. FUN-17 CSP is script-src 'self', so the
	// plugin's <script src="/plugins/sdk/v1/sdk.js"> must resolve on this
	// origin. Register before /plugins/ — ServeMux's longest-match routing
	// picks this handler for /plugins/sdk/v1/*, and the asset handler for
	// everything else under /plugins/.
	if cfg.PluginSDKDir != "" {
		publicMux.Handle("/plugins/sdk/v1/", http.StripPrefix("/plugins/sdk/v1/", http.FileServer(http.Dir(cfg.PluginSDKDir))))
		logger.Info("serving plugin-sdk v1 assets", "dir", cfg.PluginSDKDir)
	}

	// Auth for the asset handler: parse the console's UserToken cookie and
	// gate on OpenFGA can_view(user, cluster).
	assetValidator := auth.NewValidatorForAudience(
		[]byte(cfg.JWTSecret),
		auth.ConsoleAuthCookieName,
		auth.ConsoleIssuer,
		auth.TokenTypeUser,
		logger,
	)
	openfga, err := openfgaauthz.New(cfg.OpenFGA)
	if err != nil {
		return fmt.Errorf("openfga client: %w", err)
	}

	// Plugin assets: /clusters/{clusterID}/plugins/{name}/{version}/console/{path}.
	// The console picks the cluster the user is browsing, so asset traffic
	// stays local to that cluster instead of piling onto one arbitrary
	// asset-source cluster across the whole estate.
	publicMux.Handle("/clusters/", assets.NewHandler(fetcher, cfgCsp, assetValidator, &canViewAdapter{client: openfga}, logger))

	// Installation proxy (cross-site → wrap in CORS). The PluginToken rides
	// in Authorization, so credentials mode stays off — do not enable
	// AllowCredentials.
	installHandler := installproxy.New([]byte(cfg.JWTSecret), authz, backend, logger)
	installCORS := cors.New(cors.Options{
		AllowedOrigins: []string{cfg.PluginProxyOrigin},
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	})
	publicMux.Handle("/installations/", installCORS.Handler(installHandler))

	var clusterAccess service.ClusterAccess
	if cfg.PluginSandboxKubeconfig != "" {
		sandboxAccess, err := service.NewSandboxClusterAccess(cfg.PluginSandboxKubeconfig)
		if err != nil {
			return fmt.Errorf("build sandbox cluster access: %w", err)
		}
		clusterAccess = sandboxAccess
		logger.Info("PluginInstallationService reading from plugin sandbox cluster",
			"kubeconfig", cfg.PluginSandboxKubeconfig)
	} else {
		clusterAccess = service.NewMockClusterAccess()
	}
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

// canViewAdapter bridges common/authz.Client to assets.ClusterViewChecker.
type canViewAdapter struct{ client *openfgaauthz.Client }

func (a *canViewAdapter) CanViewCluster(ctx context.Context, userID, clusterID uuid.UUID) (bool, error) {
	if a.client == nil {
		return false, errors.New("openfga client not configured")
	}
	dec, err := a.client.Evaluate(ctx, openfgaauthz.EvaluationRequest{
		Subject:  openfgaauthz.User(userID),
		Action:   openfgaauthz.CanView(),
		Resource: openfgaauthz.Cluster(clusterID),
	})
	if err != nil {
		return false, err
	}
	return dec.Decision, nil
}
