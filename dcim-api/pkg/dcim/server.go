package dcim

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/connectlimit"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/dcim-api/pkg/metal"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
)

type Server struct {
	logger        *slog.Logger
	db            *psqldb.DB
	queries       *db.Queries
	authValidator *auth.Validator
	// metal reads metal-stack's series; nil while no store is configured.
	metal   *metal.Reader
	handler http.Handler
}

// Option configures a Server.
type Option func(*Server)

// WithMetal sets the reader behind MetalService. Without it the service
// answers FailedPrecondition: this environment has no metal-stack metrics.
func WithMetal(reader *metal.Reader) Option {
	return func(s *Server) {
		s.metal = reader
	}
}

func New(logger *slog.Logger, database *psqldb.DB, jwtSecret []byte, opts ...Option) *Server {
	s := &Server{
		logger:        logger,
		db:            database,
		queries:       db.New(database.Pool),
		authValidator: auth.NewValidatorForAudience(jwtSecret, auth.DCIMAuthCookieName, auth.DCIMIssuer, auth.TokenTypeDCIMUser, logger),
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

	// TODO(FUN-17): when dcim-api gains JWT validation it MUST build its
	// validator with auth.NewValidatorForAudience(secret, auth.TokenTypeUser, …)
	// so a fundament-plugin token cannot be replayed against this surface.
	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		s.authInterceptor(),
		loggingInterceptor,
		validate.NewInterceptor(),
	)

	mux.Handle(dcimv1connect.NewSiteServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewRoomServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewRackRowServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewRackServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewAssetServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewPlacementServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewPhysicalConnectionServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewCatalogServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewLogicalDesignServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewLogicalDeviceServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewLogicalConnectionServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewLogicalDeviceLayoutServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewNoteServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewTaskServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewTaskStepServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewUserServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))
	mux.Handle(dcimv1connect.NewMetalServiceHandler(s, interceptors, connectlimit.ReadMaxBytes()))

	reflector := grpcreflect.NewStaticReflector(
		"dcim.v1.SiteService",
		"dcim.v1.RoomService",
		"dcim.v1.RackRowService",
		"dcim.v1.RackService",
		"dcim.v1.AssetService",
		"dcim.v1.PlacementService",
		"dcim.v1.PhysicalConnectionService",
		"dcim.v1.CatalogService",
		"dcim.v1.LogicalDesignService",
		"dcim.v1.LogicalDeviceService",
		"dcim.v1.LogicalConnectionService",
		"dcim.v1.LogicalDeviceLayoutService",
		"dcim.v1.NoteService",
		"dcim.v1.TaskService",
		"dcim.v1.TaskStepService",
		"dcim.v1.UserService",
		"dcim.v1.MetalService",
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector, connectlimit.ReadMaxBytes()))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector, connectlimit.ReadMaxBytes()))

	s.handler = mux

	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
