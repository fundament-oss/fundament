// Package fleet serves fleet.v1.FleetService, the staff view of every
// organization's clusters (FUN-23).
package fleet

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1/fleetv1connect"
)

// staleAfterIntervals is how many inventory intervals a snapshot may miss
// before it counts as stale. The interval is the one the inventory loop
// recorded with its last pass, so the rule follows its deployment.
const staleAfterIntervals = 3

type Config struct {
	JWTSecret          []byte
	CORSAllowedOrigins []string
}

type Server struct {
	logger        *slog.Logger
	queries       *db.Queries
	authValidator *auth.Validator
	clock         func() time.Time
	handler       http.Handler
}

// New serves the fleet view over a pool connected as fun_platform_admin_api,
// whose policies read every organization and whose grants write nothing.
func New(logger *slog.Logger, cfg Config, database *psqldb.DB) *Server {
	s := &Server{
		logger:  logger,
		queries: db.New(database.Pool),
		// The credential is the DCIM token: the viewers are Fundament staff,
		// who sign in through the staff IDP (dexDcim via dcim-authn-api), as for
		// marketplace review (FUN-20). Pinned to the dcim-user audience as in
		// dcim-api, so no other token signed with the shared secret passes.
		authValidator: auth.NewValidatorForAudience(
			cfg.JWTSecret, auth.DCIMAuthCookieName, auth.DCIMIssuer, auth.TokenTypeDCIMUser, logger,
		),
		clock: time.Now,
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

// heartbeats are the last passes of cluster-worker's periodic loops.
type heartbeats struct {
	reconcile, inventory *db.WorkerHeartbeatListRow
}

func (s *Server) heartbeats(ctx context.Context) (heartbeats, error) {
	rows, err := s.queries.WorkerHeartbeatList(ctx)
	if err != nil {
		return heartbeats{}, fmt.Errorf("list worker heartbeats: %w", err)
	}
	var h heartbeats
	for i := range rows {
		switch dbconst.WorkerHeartbeatWorker(rows[i].Worker) {
		case dbconst.WorkerHeartbeatWorker_Reconcile:
			h.reconcile = &rows[i]
		case dbconst.WorkerHeartbeatWorker_Inventory:
			h.inventory = &rows[i]
		default:
			panic(fmt.Sprintf("unhandled worker %q", rows[i].Worker))
		}
	}
	return h, nil
}

// staleAfter is how old a snapshot may get, or zero while the inventory loop
// has not run.
func (h heartbeats) staleAfter() time.Duration {
	if h.inventory == nil {
		return 0
	}
	return staleAfterIntervals * time.Duration(h.inventory.IntervalSeconds) * time.Second
}

// staleBefore is the cutoff a snapshot must be newer than; NULL, so nothing is
// stale, while the inventory loop has not run.
func (s *Server) staleBefore(h heartbeats) pgtype.Timestamptz {
	if h.inventory == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: s.clock().Add(-h.staleAfter()), Valid: true}
}
