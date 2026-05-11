package dcim

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
)

type Server struct {
	logger  *slog.Logger
	db      *psqldb.DB
	queries *db.Queries
	handler http.Handler
}

func New(logger *slog.Logger, database *psqldb.DB) *Server {
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

	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		loggingInterceptor,
		validate.NewInterceptor(),
	)

	mux.Handle(dcimv1connect.NewSiteServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewRoomServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewRackRowServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewRackServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewAssetServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewPlacementServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewPhysicalConnectionServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewCatalogServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewLogicalDesignServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewLogicalDeviceServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewLogicalConnectionServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewLogicalDeviceLayoutServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewNoteServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewTaskServiceHandler(s, interceptors))
	mux.Handle(dcimv1connect.NewTaskStepServiceHandler(s, interceptors))

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
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	s.handler = mux

	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
