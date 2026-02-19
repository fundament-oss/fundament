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
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/organization-api/pkg/clock"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
)

type Config struct {
	JWTSecret          []byte
	CORSAllowedOrigins []string
	Clock              clock.Clock
}

type Server struct {
	config        *Config
	db            *psqldb.DB
	queries       *db.Queries
	logger        *slog.Logger
	authValidator *auth.Validator
	authz         *authz.Client
	clock         clock.Clock
	handler       http.Handler
}

func New(logger *slog.Logger, cfg *Config, database *psqldb.DB, authzClient *authz.Client) (*Server, error) {
	clk := cfg.Clock
	if clk == nil {
		clk = clock.New()
	}

	s := &Server{
		logger:        logger,
		config:        cfg,
		db:            database,
		queries:       db.New(database.Pool),
		authValidator: auth.NewValidator(cfg.JWTSecret, logger),
		authz:         authzClient,
		clock:         clk,
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
		s.authInterceptor(),
		validate.NewInterceptor(),
		loggingInterceptor,
	)

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

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "Connect-Protocol-Version", "Fun-Organization"},
		AllowCredentials: true,
	})

	s.handler = corsHandler.Handler(mux)

	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
