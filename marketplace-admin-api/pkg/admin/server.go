package admin

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
	db "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/db/gen"
	adminv1connect "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1/adminv1connect"
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
		// The reviewer credential is the DCIM token: reviewers are Fundament
		// staff, who authenticate against the staff IDP (dexDcim via
		// dcim-authn-api), not the customer-facing console. DCIM tokens carry
		// no audience, so this cannot use NewValidatorForAudience; the issuer
		// check alone rejects console user tokens and plugin tokens (FUN-17),
		// both issued by fundament-authn-api.
		authValidator: auth.NewValidator(
			cfg.JWTSecret, auth.DCIMAuthCookieName, auth.DCIMIssuer, logger,
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

	mux.Handle(adminv1connect.NewReviewServiceHandler(s, interceptors))

	// Unauthenticated, as on every other surface here: the descriptors are
	// public and grpcurl carries no token.
	reflector := grpcreflect.NewStaticReflector("admin.v1.ReviewService")
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// No organization header: this surface is authenticated but never
	// organization-scoped.
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent"},
		ExposedHeaders:   []string{"Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin"},
		AllowCredentials: true,
	})

	s.handler = corsHandler.Handler(mux)

	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
