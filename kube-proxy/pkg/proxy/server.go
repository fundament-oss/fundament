package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"

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
	kubeClient    kube.Interface         // non-nil in mock mode
	kubeProxy     *httputil.ReverseProxy // non-nil in real mode
	handler       http.Handler
}

func New(logger *slog.Logger, cfg *Config, authzClient *authz.Client) (*Server, error) {
	if cfg.Mode == "" {
		cfg.Mode = "mock"
	}
	if cfg.Mode != "mock" && cfg.Mode != "real" {
		return nil, fmt.Errorf(`invalid Mode %q: must be "mock" or "real"`, cfg.Mode)
	}

	var (
		kubeClient kube.Interface
		kubeProxy  *httputil.ReverseProxy
	)
	if cfg.Mode == "real" {
		c, err := kube.New(cfg.KubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("create kube client: %w", err)
		}
		target := c.Host()
		kubeProxy = &httputil.ReverseProxy{
			Transport: c.Transport(),
			Director: func(req *http.Request) {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.Host = target.Host
				// Strip client auth headers so the kubeconfig transport supplies its own.
				req.Header.Del("Authorization")
				req.Header.Del("Cookie")
				req.Header.Del(OrganizationHeader)
				req.Header.Del(ClusterHeader)
			},
			ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
				logger.ErrorContext(req.Context(), "kubernetes proxy error", "error", err)
				http.Error(w, "failed to contact kubernetes API", http.StatusBadGateway)
			},
		}
	} else {
		kubeClient = &kube.MockClient{}
	}

	s := &Server{
		logger:        logger,
		authValidator: auth.NewValidator(cfg.JWTSecret, logger),
		authz:         authzClient,
		kubeClient:    kubeClient,
		kubeProxy:     kubeProxy,
	}

	mux := http.NewServeMux()
	mux.Handle("/k8sproxy/", http.HandlerFunc(s.handleClusterProxy)) // cluster ID via Fun-Cluster header
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
		AllowedHeaders:   []string{"Content-Type", "Authorization", OrganizationHeader, ClusterHeader},
		AllowCredentials: true,
	})

	s.handler = corsHandler.Handler(mux)

	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
