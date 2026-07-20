package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/rs/cors"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/pluginsa"
	tokenpkg "github.com/fundament-oss/fundament/kube-api-proxy/pkg/token"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/useraccess"
)

type Config struct {
	JWTSecret          []byte
	CORSAllowedOrigins []string
	Mode               string // "mock" (default) or "real"
	GardenerClient     *gardener.Client
	// MockPluginTemplatesDir is the on-disk root from which `/proxy/console/*`
	// requests are answered in mock mode. Layout: <dir>/<pluginName>/console/<file>.
	// Ignored in "real" mode.
	MockPluginTemplatesDir string
	// PluginSandboxKubeconfig, when set in mock mode, replaces MockClient with
	// a proxy that forwards every request to a locally-running plugin sandbox
	// cluster identified by the kubeconfig at this path. Ignored otherwise.
	PluginSandboxKubeconfig string
}

type Server struct {
	logger        *slog.Logger
	authValidator *auth.Validator
	authz         *authz.Client
	tokenCache    *tokenpkg.Cache
	kubeHandler   http.Handler
	handler       http.Handler
	pluginGateway *pluginGateway
}

func New(logger *slog.Logger, cfg *Config, authzClient *authz.Client) (*Server, error) {
	if cfg.Mode == "" {
		cfg.Mode = "mock"
	}

	var kubeHandler http.Handler
	var tokenCache *tokenpkg.Cache
	switch cfg.Mode {
	case "real":
		tokenCache = tokenpkg.NewCache(cfg.GardenerClient, logger)
		kubeHandler = kube.NewMultiClusterProxy(cfg.GardenerClient, logger)
	case "mock":
		if cfg.PluginSandboxKubeconfig != "" {
			sandbox, err := kube.NewSandboxProxy(cfg.PluginSandboxKubeconfig, logger)
			if err != nil {
				return nil, fmt.Errorf("build plugin sandbox proxy: %w", err)
			}
			kubeHandler = sandbox
			logger.Info("mock mode: proxying kube API requests to plugin sandbox cluster",
				"kubeconfig", cfg.PluginSandboxKubeconfig)
		} else {
			kubeHandler = &kube.MockClient{PluginTemplatesDir: cfg.MockPluginTemplatesDir}
		}
	default:
		return nil, fmt.Errorf("invalid Mode %q: must be \"mock\" or \"real\"", cfg.Mode)
	}

	s := &Server{
		logger:        logger,
		authValidator: auth.NewValidatorForAudience(cfg.JWTSecret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, logger),
		authz:         authzClient,
		tokenCache:    tokenCache,
		kubeHandler:   kubeHandler,
	}

	pluginSA, err := newPluginSAResolver(cfg, logger)
	if err != nil {
		return nil, err
	}

	userSAR, err := newUserAccessChecker(cfg, logger)
	if err != nil {
		return nil, err
	}

	s.pluginGateway = &pluginGateway{
		logger:      logger,
		jwtSecret:   cfg.JWTSecret,
		userSAR:     userSAR,
		pluginSA:    pluginSA,
		canView:     authzClient.CanViewCluster,
		kubeHandler: kubeHandler,
	}

	mux := http.NewServeMux()
	// Catch-all pattern for cluster-scoped requests.
	// The handler validates that the remaining path starts with an allowed
	// Kubernetes API prefix (api, apis, openapi/) before forwarding.
	mux.Handle("/clusters/{clusterID}/{path...}", http.HandlerFunc(s.handleClusterProxy))
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := s.authz.Healthy(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("openfga: " + err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	corsOpts := cors.Options{
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}
	if len(cfg.CORSAllowedOrigins) > 0 {
		corsOpts.AllowedOrigins = cfg.CORSAllowedOrigins
	} else {
		// AllowedOrigins=["*"] with AllowCredentials=true is rejected by browsers.
		// Reflect the request origin instead.
		corsOpts.AllowOriginFunc = func(_ string) bool { return true }
	}
	corsHandler := cors.New(corsOpts)

	s.handler = s.requestLogger(corsHandler.Handler(mux))

	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func newUserAccessChecker(cfg *Config, logger *slog.Logger) (useraccess.Checker, error) {
	if cfg.Mode == "mock" {
		if cfg.PluginSandboxKubeconfig == "" {
			// Pure mock: no real cluster to review against, so allow all.
			return useraccess.Stub{}, nil
		}

		// Sandbox: run the user half against the same sandbox apiserver the
		// plugin half mints tokens on, so the FUN-17 user∩plugin intersection
		// is actually enforced (and exercised) locally instead of silently
		// skipped by the allow-all Stub.
		client, err := useraccess.NewSandboxClient(cfg.PluginSandboxKubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build sandbox useraccess client: %w", err)
		}
		return useraccess.New(client, logger), nil
	}
	return useraccess.New(cfg.GardenerClient, logger), nil
}

func newPluginSAResolver(cfg *Config, logger *slog.Logger) (pluginsa.Resolver, error) {
	if cfg.Mode == "mock" {
		if cfg.PluginSandboxKubeconfig == "" {
			// Pure mock: MockClient doesn't check the bearer, so a stub token is fine.
			return pluginsa.Stub{}, nil
		}

		// Sandbox: forward to a real apiserver, so mint real TokenRequests
		// against the sandbox kubeconfig — the same code path prod uses,
		// enforced by the sandbox cluster's RBAC on the plugin SA.
		client, err := pluginsa.NewSandboxClient(cfg.PluginSandboxKubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build sandbox pluginsa client: %w", err)
		}

		return pluginsa.New(client, logger), nil
	}

	return pluginsa.New(cfg.GardenerClient, logger), nil
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"origin", r.Header.Get("Origin"),
			"acl-request-method", r.Header.Get("Access-Control-Request-Method"),
			"acl-allow-origin", w.Header().Get("Access-Control-Allow-Origin"),
		)
	})
}
