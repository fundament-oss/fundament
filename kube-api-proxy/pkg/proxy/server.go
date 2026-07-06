package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/cors"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
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

	s.pluginGateway = &pluginGateway{
		logger:      logger,
		jwtSecret:   cfg.JWTSecret,
		userSAR:     newUserAccessChecker(cfg),
		pluginSA:    newPluginSAResolver(cfg),
		canView:     &canViewAdapter{client: authzClient},
		kubeHandler: kubeHandler,
	}
	if cfg.Mode == "real" {
		logger.Warn("plugin gateway installed with unwired real-mode stubs; PluginToken requests will fail-closed until FUN-17 SAR and plugin-SA resolver wiring lands")
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

// canViewAdapter adapts common/authz.Client to the ClusterViewChecker
// interface the plugin gateway needs.
type canViewAdapter struct{ client *authz.Client }

func (a *canViewAdapter) CanViewCluster(ctx context.Context, userID, clusterID uuid.UUID) (bool, error) {
	if a.client == nil {
		return false, errors.New("openfga client not configured")
	}
	dec, err := a.client.Evaluate(ctx, authz.EvaluationRequest{
		Subject:  authz.User(userID),
		Action:   authz.CanView(),
		Resource: authz.Cluster(clusterID),
	})
	if err != nil {
		return false, fmt.Errorf("openfga can_view: %w", err)
	}
	return dec.Decision, nil
}

// stubUserAccessChecker is the mock-mode UserAccessChecker: allow-all. The real
// implementation issues an authorization.k8s.io/v1 SubjectAccessReview against
// the target cluster with the per-user SA username as Spec.User — a follow-up
// coordinated with the Gardener wiring in real mode.
type stubUserAccessChecker struct{}

func (stubUserAccessChecker) Check(_ context.Context, _ string, _ *kubereq.Attributes, _ string) (bool, error) {
	return true, nil
}

// unwiredUserAccessChecker is the real-mode placeholder until the SAR wiring
// lands. It fail-closes every request so a running proxy cannot silently
// allow-all on the plugin path.
type unwiredUserAccessChecker struct{}

func (unwiredUserAccessChecker) Check(_ context.Context, _ string, _ *kubereq.Attributes, _ string) (bool, error) {
	return false, errors.New("user SAR checker not wired for real mode")
}

// stubPluginSAResolver is the mock-mode PluginSAResolver: returns a canned
// token so plugin-token requests can be exercised without a real cluster or
// PluginInstallation informer.
type stubPluginSAResolver struct{}

func (stubPluginSAResolver) Resolve(_ context.Context, _, _ string) (PluginSA, error) {
	return PluginSA{Token: "mock-plugin-sa-token", PinnedDefinitionHash: "sha256:mock"}, nil //nolint:gosec // mock token for tests; real resolver uses TokenRequest against the target cluster
}

// unwiredPluginSAResolver is the real-mode placeholder until the informer +
// TokenRequest wiring lands. Fail-closed so no request forwards without a real
// plugin SA token.
type unwiredPluginSAResolver struct{}

func (unwiredPluginSAResolver) Resolve(_ context.Context, _, _ string) (PluginSA, error) {
	return PluginSA{}, errors.New("plugin SA resolver not wired for real mode")
}

func newUserAccessChecker(cfg *Config) UserAccessChecker {
	if cfg.Mode == "mock" {
		return stubUserAccessChecker{}
	}
	// FUN-17: real-mode wiring issues a SubjectAccessReview against the target
	// cluster with Spec.User = "system:serviceaccount:{ns}/fundament-{userID}".
	// Until that lands, deny.
	return unwiredUserAccessChecker{}
}

func newPluginSAResolver(cfg *Config) PluginSAResolver {
	if cfg.Mode == "mock" {
		return stubPluginSAResolver{}
	}
	// FUN-17: real-mode wiring reads PluginInstallation from a local informer
	// for the pinned definition hash, and obtains the plugin SA token via a
	// short-lived TokenRequest against the target cluster. Until that lands,
	// deny.
	return unwiredPluginSAResolver{}
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
