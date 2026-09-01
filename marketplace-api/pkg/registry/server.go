package registry

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/marketplace-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/registry/v1/registryv1connect"
)

type Config struct {
	JWTSecret          []byte
	CORSAllowedOrigins []string
}

type Server struct {
	logger        *slog.Logger
	db            *psqldb.DB
	queries       *db.Queries
	authValidator *auth.Validator
	handler       http.Handler
}

func New(logger *slog.Logger, cfg Config, database *psqldb.DB) *Server {
	s := &Server{
		logger:  logger,
		db:      database,
		queries: db.New(database.Pool),
		// NewValidatorForAudience, not NewValidator: a PluginToken must not be
		// accepted here (FUN-17).
		authValidator: auth.NewValidatorForAudience(
			cfg.JWTSecret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, logger,
		),
	}

	mux := http.NewServeMux()

	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)

	// Auth runs before validate so an unauthenticated caller is turned away
	// before the request body is inspected.
	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		s.authInterceptor(),
		validate.NewInterceptor(),
		loggingInterceptor,
	)

	mux.Handle(registryv1connect.NewPublicationServiceHandler(s, interceptors))

	reflector := grpcreflect.NewStaticReflector("registry.v1.PublicationService")
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// Wider than the catalog's: this surface is authenticated, so the developer
	// frontend sends credentials plus the organization header.
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent", OrganizationHeader},
		ExposedHeaders:   []string{"Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin"},
		AllowCredentials: true,
	})

	s.handler = corsHandler.Handler(mux)

	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
