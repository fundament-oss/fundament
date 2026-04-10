package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
	"github.com/rs/cors"
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
	kubeHandler   http.Handler
	handler       http.Handler
}

func New(logger *slog.Logger, cfg *Config, authzClient *authz.Client) (*Server, error) {
	if cfg.Mode == "" {
		cfg.Mode = "mock"
	}

	var kubeHandler http.Handler
	switch cfg.Mode {
	case "real":
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
		kubeHandler:   kubeHandler,
	}

	mux := http.NewServeMux()
	// /api/ = core resources (Pods, Services, …)
	// /apis/ = API groups (apps, CRDs, …)
	// Both prefixes are registered explicitly to prevent SSRF — only these paths reach the proxy
	mux.Handle("/apis/", http.HandlerFunc(s.handleClusterProxy))
	mux.Handle("/api/", http.HandlerFunc(s.handleClusterProxy))
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

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", OrganizationHeader, ClusterHeader},
		AllowCredentials: true,
	})

	s.handler = corsHandler.Handler(mux)

	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
