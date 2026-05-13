package organization

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/circuitbreaker"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/idempotency"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/organization-api/pkg/clock"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
)

type Config struct {
	JWTSecret            []byte
	CORSAllowedOrigins   []string
	Clock                clock.Clock
	MockPrometheusClient *prom.MockClient
	PrometheusURL        string // Prometheus URL for metrics; "mock" uses generated data
	KubeAPIProxyURL      string // Base URL for the kube-api-proxy (e.g. "https://kube-proxy.fundament.example")
}

type Server struct {
	config         *Config
	db             *psqldb.DB
	queries        *db.Queries
	logger         *slog.Logger
	authValidator  *auth.Validator
	authz          *authz.Client
	circuitBreaker *circuitbreaker.Breaker
	clock          clock.Clock
	handler        http.Handler
	mockPromClient *prom.MockClient
	prometheusURL  string
}

// Option configures optional Server dependencies.
type Option func(*Server)

// WithCircuitBreaker adds a circuit breaker that blocks all requests when tripped.
func WithCircuitBreaker(b *circuitbreaker.Breaker) Option {
	return func(s *Server) {
		s.circuitBreaker = b
	}
}

func New(logger *slog.Logger, cfg *Config, database *psqldb.DB, authzClient *authz.Client, idempotencyStore *idempotency.Store, opts ...Option) (*Server, error) {
	clk := cfg.Clock
	if clk == nil {
		clk = clock.New()
	}

	s := &Server{
		logger:         logger,
		config:         cfg,
		db:             database,
		queries:        db.New(database.Pool),
		authValidator:  auth.NewValidatorForAudience(cfg.JWTSecret, auth.TokenTypeUser, logger),
		authz:          authzClient,
		clock:          clk,
		mockPromClient: cfg.MockPrometheusClient,
		prometheusURL:  cfg.PrometheusURL,
	}

	for _, opt := range opts {
		opt(s)
	}

	mux := http.NewServeMux()
	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)

	procedures := buildProcedures(s.queries)

	chain := []connect.Interceptor{
		connectrecovery.NewInterceptor(logger),
	}

	// Circuit breaker is placed before auth so that open-breaker requests
	// are rejected early with CodeUnavailable, avoiding unnecessary auth work.
	if s.circuitBreaker != nil {
		chain = append(chain, circuitbreaker.NewInterceptor(s.circuitBreaker))
	}

	chain = append(chain,
		s.authInterceptor(),
		validate.NewInterceptor(),
		loggingInterceptor,
		idempotency.NewInterceptor(logger, idempotencyStore, UserIDFromContext, procedures),
	)
	interceptors := connect.WithInterceptors(chain...)

	orgPath, orgHandler := organizationv1connect.NewOrganizationServiceHandler(s, interceptors)
	mux.Handle(orgPath, orgHandler)

	clusterPath, clusterHandler := organizationv1connect.NewClusterServiceHandler(s, interceptors)
	mux.Handle(clusterPath, clusterHandler)

	pluginPath, pluginHandler := organizationv1connect.NewPluginServiceHandler(s, interceptors)
	mux.Handle(pluginPath, pluginHandler)

	// gRPC reflection for API discovery (used by Bruno, grpcurl, etc.)
	reflector := grpcreflect.NewStaticReflector(
		"organization.v1.OrganizationService",
		"organization.v1.ClusterService",
		"organization.v1.PluginService",
		"organization.v1.MemberService",
		"organization.v1.InviteService",
		"organization.v1.APIKeyService",
		"organization.v1.NamespaceService",
		"organization.v1.MetricsService",
	)
	reflectPath, reflectHandler := grpcreflect.NewHandlerV1(reflector)
	mux.Handle(reflectPath, reflectHandler)
	reflectPathAlpha, reflectHandlerAlpha := grpcreflect.NewHandlerV1Alpha(reflector)
	mux.Handle(reflectPathAlpha, reflectHandlerAlpha)

	projectPath, projectHandler := organizationv1connect.NewProjectServiceHandler(s, interceptors)
	mux.Handle(projectPath, projectHandler)

	namespacePath, namespaceHandler := organizationv1connect.NewNamespaceServiceHandler(s, interceptors)
	mux.Handle(namespacePath, namespaceHandler)

	memberPath, memberHandler := organizationv1connect.NewMemberServiceHandler(s, interceptors)
	mux.Handle(memberPath, memberHandler)

	invitePath, inviteHandler := organizationv1connect.NewInviteServiceHandler(s, interceptors)
	mux.Handle(invitePath, inviteHandler)

	apiKeyPath, apiKeyHandler := organizationv1connect.NewAPIKeyServiceHandler(s, interceptors)
	mux.Handle(apiKeyPath, apiKeyHandler)

	metricsPath, metricsHandler := organizationv1connect.NewMetricsServiceHandler(s, interceptors)
	mux.Handle(metricsPath, metricsHandler)

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent", "Fun-Organization", idempotency.HeaderIdempotencyKey},
		ExposedHeaders:   []string{"Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin", idempotency.HeaderIdempotencyStatus},
		AllowCredentials: true,
	})

	s.handler = corsHandler.Handler(mux)

	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
