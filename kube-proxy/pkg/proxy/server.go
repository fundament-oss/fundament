package proxy

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-proxy/pkg/kube"
	"github.com/rs/cors"
)

type Config struct {
	JWTSecret          []byte
	CORSAllowedOrigins []string
	Mode               string // "mock" (default) or "real"
	KubeconfigPath     string // path to kubeconfig; only used when Mode == "real"
}

type Server struct {
	logger        *slog.Logger
	authValidator *auth.Validator
	authz         *authz.Client
	kubeClient    kube.Interface
	handler       http.Handler
}

func newKubeClient(cfg *Config) (kube.Interface, error) {
	if cfg.Mode == "real" {
		return kube.New(cfg.KubeconfigPath)
	}
	return &kube.MockClient{}, nil
}

func New(logger *slog.Logger, cfg *Config, authzClient *authz.Client) (*Server, error) {
	if cfg.Mode == "" {
		cfg.Mode = "mock"
	}
	if cfg.Mode != "mock" && cfg.Mode != "real" {
		return nil, fmt.Errorf(`invalid Mode %q: must be "mock" or "real"`, cfg.Mode)
	}

	kubeClient, err := newKubeClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kube client: %w", err)
	}

	s := &Server{
		logger:        logger,
		authValidator: auth.NewValidator(cfg.JWTSecret, logger),
		authz:         authzClient,
		kubeClient:    kubeClient,
	}

	mux := http.NewServeMux()
	mux.Handle("/k8sproxy/", http.HandlerFunc(s.handleClusterProxy))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", OrganizationHeader},
		AllowCredentials: true,
	})

	s.handler = corsHandler.Handler(mux)

	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
