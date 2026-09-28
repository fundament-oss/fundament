package catalog

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1/catalogv1connect"
	_ "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/install/v1" // registers install.v1 descriptors for reflection
)

type Server struct {
	logger  *slog.Logger
	db      *psqldb.DB
	queries *db.Queries
	handler http.Handler
}

// NewService returns the read handlers over the given pool without an HTTP
// handler. install.v1 serves the same RPCs through a pool connected as the
// tenant-aware install role (FUN-22); the policies, not the code, differ.
func NewService(logger *slog.Logger, database *psqldb.DB) *Server {
	return &Server{
		logger:  logger,
		db:      database,
		queries: db.New(database.Pool),
	}
}

func New(logger *slog.Logger, database *psqldb.DB, corsAllowedOrigins []string) *Server {
	s := NewService(logger, database)

	mux := http.NewServeMux()

	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)

	// No auth interceptor: the storefront is unauthenticated by design (FUN-20).
	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		loggingInterceptor,
		validate.NewInterceptor(),
	)

	mux.Handle(catalogv1connect.NewCatalogServiceHandler(s, interceptors))

	// install.v1 is served from this binary too (see pkg/install) and shares the
	// reflection endpoint: descriptors come from the global registry.
	reflector := grpcreflect.NewStaticReflector("catalog.v1.CatalogService", "install.v1.InstallService")
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// The storefront is a browser client calling this directly. Narrower than
	// organization-api's: no credentials, so no Authorization, Fun-Organization
	// or idempotency headers.
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   corsAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent"},
		ExposedHeaders:   []string{"Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin"},
		AllowCredentials: false,
	})

	s.handler = corsHandler.Handler(mux)

	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
