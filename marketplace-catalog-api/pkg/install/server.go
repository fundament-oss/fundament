package install

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/catalog"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/install/v1/installv1connect"
)

type Config struct {
	JWTSecret          []byte
	CORSAllowedOrigins []string
}

// Server serves install.v1. Service is the storefront's handler set over the
// install pool: install.v1 reuses catalog.v1's messages, so the storefront's
// handlers serve it, with ListPublishers widened to what ListPlugins returns.
type Server struct {
	logger        *slog.Logger
	jwtSecret     []byte
	userValidator *auth.Validator
	Service       *catalog.InstallServer
	path          string
	handler       http.Handler
}

// New builds the install surface over a pool from NewDB.
func New(logger *slog.Logger, cfg Config, database *psqldb.DB) *Server {
	s := &Server{
		logger:    logger,
		jwtSecret: cfg.JWTSecret,
		// Audience-pinned: a PluginToken or WorkloadToken must not pass as a
		// user here; workload tokens take their own path in authenticate.
		userValidator: auth.NewValidatorForAudience(
			cfg.JWTSecret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, logger,
		),
		Service: catalog.NewInstallService(catalog.NewService(logger, database)),
	}

	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)

	// Auth before validate: an unauthenticated caller is turned away before
	// the request body is inspected.
	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		auth.NewInterceptor(s.authenticate),
		validate.NewInterceptor(),
		loggingInterceptor,
	)

	path, handler := installv1connect.NewInstallServiceHandler(s.Service, interceptors)

	// Credentialed, unlike the storefront's: the console sends its cookie or
	// bearer plus the organization header. Same shape as the registry's.
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent", OrganizationHeader},
		ExposedHeaders:   []string{"Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin"},
		AllowCredentials: true,
	})

	s.path = path
	s.handler = corsHandler.Handler(handler)
	return s
}

// Path is the mux pattern the service answers on (/install.v1.InstallService/).
func (s *Server) Path() string {
	return s.path
}

// Handler serves install.v1 with its own CORS policy; mount it at Path().
func (s *Server) Handler() http.Handler {
	return s.handler
}
