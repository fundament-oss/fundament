// Package authzapi answers which actions the caller may take. It depends only
// on OpenFGA.
package authzapi

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	authzv1connect "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1/authzv1connect"
	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/connectrecovery"
)

type Config struct {
	JWTSecret          []byte
	CORSAllowedOrigins []string
}

type Server struct {
	logger        *slog.Logger
	authValidator *auth.Validator
	authz         *authz.Client
	handler       http.Handler
}

func New(logger *slog.Logger, cfg *Config, authzClient *authz.Client) *Server {
	s := &Server{
		logger:        logger,
		authValidator: auth.NewValidatorForAudience(cfg.JWTSecret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, logger),
		authz:         authzClient,
	}

	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)

	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		auth.NewInterceptor(s.authenticate),
		validate.NewInterceptor(),
		loggingInterceptor,
	)

	mux := http.NewServeMux()

	path, handler := authzv1connect.NewAuthzServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	reflector := grpcreflect.NewStaticReflector(authzv1connect.AuthzServiceName)
	reflectPath, reflectHandler := grpcreflect.NewHandlerV1(reflector)
	mux.Handle(reflectPath, reflectHandler)
	reflectPathAlpha, reflectHandlerAlpha := grpcreflect.NewHandlerV1Alpha(reflector)
	mux.Handle(reflectPathAlpha, reflectHandlerAlpha)

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

// authenticate puts the caller's id from the validated JWT into ctx. Nothing
// else identifies the caller.
func (s *Server) authenticate(ctx context.Context, _ string, header http.Header) (context.Context, error) {
	claims, err := s.authValidator.Validate(header)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	return auth.WithUserID(ctx, claims.UserID()), nil
}
