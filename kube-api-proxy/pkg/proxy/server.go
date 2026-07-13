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
	tokenpkg "github.com/fundament-oss/fundament/kube-api-proxy/pkg/token"
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
	// ConsoleOrigins are the origins the Console is served from. They are the only
	// origins a plugin console asset may be bootstrapped from, and the only ones its
	// CSP admits scripts from — see kube.ConsoleAssetPolicy. Empty disables both
	// checks (bare local dev); every deployed environment sets it.
	ConsoleOrigins []string
	// PublicOrigin is this proxy's own public origin, i.e. the origin plugin console
	// assets are served from. Named in their CSP alongside ConsoleOrigins.
	PublicOrigin string
}

type Server struct {
	logger        *slog.Logger
	authValidator *auth.Validator
	authz         *authz.Client
	tokenCache    *tokenpkg.Cache
	kubeHandler   http.Handler
	handler       http.Handler
	consoleAssets kube.ConsoleAssetPolicy
}

func New(logger *slog.Logger, cfg *Config, authzClient *authz.Client) (*Server, error) {
	if cfg.Mode == "" {
		cfg.Mode = "mock"
	}

	consoleAssets := kube.ConsoleAssetPolicy{
		AssetOrigin:    kube.NormalizeOrigin(cfg.PublicOrigin),
		ConsoleOrigins: kube.NormalizeOrigins(cfg.ConsoleOrigins),
	}
	if len(consoleAssets.ConsoleOrigins) == 0 {
		logger.Warn("CONSOLE_ORIGINS is not set: plugin console assets are served without a " +
			"Content-Security-Policy and with an unchecked ?host= origin. Set it to the origin(s) " +
			"the Console is served from.")
	}

	var kubeHandler http.Handler
	var tokenCache *tokenpkg.Cache
	switch cfg.Mode {
	case "real":
		tokenCache = tokenpkg.NewCache(cfg.GardenerClient, logger)
		kubeHandler = kube.NewMultiClusterProxy(cfg.GardenerClient, logger)
	case "mock":
		kubeHandler = &kube.MockClient{
			PluginTemplatesDir: cfg.MockPluginTemplatesDir,
			ConsoleAssets:      consoleAssets,
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
		consoleAssets: consoleAssets,
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
