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
	db "github.com/fundament-oss/fundament/marketplace-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1/catalogv1connect"
)

type Server struct {
	logger  *slog.Logger
	db      *psqldb.DB
	queries *db.Queries
	handler http.Handler
}

func New(logger *slog.Logger, database *psqldb.DB, corsAllowedOrigins []string) *Server {
	s := &Server{
		logger:  logger,
		db:      database,
		queries: db.New(database.Pool),
	}

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

	reflector := grpcreflect.NewStaticReflector("catalog.v1.CatalogService")
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
