// Package fleet serves fleet.v1.FleetService, the staff view of every
// organization's clusters (FUN-23).
package fleet

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1/fleetv1connect"
)

// DefaultStaleAfter is how old a snapshot may get before it counts as stale:
// three missed collections at the default 15 minute interval.
const DefaultStaleAfter = 45 * time.Minute

type Config struct {
	JWTSecret          []byte
	CORSAllowedOrigins []string
	StaleAfter         time.Duration
}

type Server struct {
	logger        *slog.Logger
	queries       *db.Queries
	authValidator *auth.Validator
	staleAfter    time.Duration
	clock         func() time.Time
	handler       http.Handler
}

// New serves the fleet view over a pool connected as fun_platform_admin_api,
// whose policies read every organization and whose grants write nothing.
func New(logger *slog.Logger, cfg Config, database *psqldb.DB) *Server {
	staleAfter := cfg.StaleAfter
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	s := &Server{
		logger:  logger,
		queries: db.New(database.Pool),
		// The credential is the DCIM token: the viewers are Fundament staff,
		// who sign in through the staff IDP (dexDcim via dcim-authn-api), as for
		// marketplace review (FUN-20). DCIM tokens carry no audience; the
		// issuer check alone refuses console and plugin tokens.
		authValidator: auth.NewValidator(
			cfg.JWTSecret, auth.DCIMAuthCookieName, auth.DCIMIssuer, logger,
		),
		staleAfter: staleAfter,
		clock:      time.Now,
	}

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

	mux := http.NewServeMux()
	mux.Handle(fleetv1connect.NewFleetServiceHandler(s, interceptors))

	// Unauthenticated, as on every other surface here: the descriptors are
	// public and grpcurl carries no token.
	reflector := grpcreflect.NewStaticReflector(fleetv1connect.FleetServiceName)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// The DCIM frontend sends the dcim_auth cookie, so credentials are
	// allowed and the origin list must be explicit.
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

// staleBefore is the cutoff a snapshot must be newer than.
func (s *Server) staleBefore() time.Time {
	return s.clock().Add(-s.staleAfter)
}
