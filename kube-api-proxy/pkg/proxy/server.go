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
}

type Server struct {
	logger        *slog.Logger
	authValidator *auth.Validator
	authz         *authz.Client
	tokenCache    *tokenpkg.Cache
	kubeHandler   http.Handler
	handler       http.Handler
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
		kubeHandler = &kube.MockClient{}
	default:
		return nil, fmt.Errorf("invalid Mode %q: must be \"mock\" or \"real\"", cfg.Mode)
	}

	s := &Server{
		logger:        logger,
		authValidator: auth.NewValidator(cfg.JWTSecret, logger),
		authz:         authzClient,
		tokenCache:    tokenCache,
		kubeHandler:   kubeHandler,
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
