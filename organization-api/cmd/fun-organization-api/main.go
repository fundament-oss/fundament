package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"connectrpc.com/validate"

	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

type config struct {
	Database           psqldb.Config
	JWTSecret          string     `env:"JWT_SECRET,required,notEmpty" `
	ListenAddr         string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins []string   `env:"CORS_ALLOWED_ORIGINS"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var cfg config
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("env parse: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting organization-api",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
	)

	ctx := context.Background()

	logger.Debug("connecting to database")

	options := []psqldb.Option{
		func(ctx context.Context, config *pgxpool.Config) {
			config.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
				queries := db.New(conn)

				// Extract organization_id from context and set it in PostgreSQL session for RLS

				if organizationID, ok := organization.OrganizationIDFromContext(ctx); ok {
					logger.Debug("setting organization context for RLS", "organization_id", organizationID.String())

					params := db.SetOrganizationContextParams{
						SetConfig: organizationID.String(),
					}

					if err := queries.SetOrganizationContext(ctx, params); err != nil {
						return false, fmt.Errorf("failed to set organization context: %w", err)
					}
				} else {
					logger.Debug("no organization_id in context for PrepareConn")
				}

				// Extract user_id from context and set it in PostgreSQL session for RLS

				if userID, ok := organization.UserIDFromContext(ctx); ok {
					logger.Debug("setting user context for RLS", "user_id", userID.String())

					params := db.SetUserContextParams{
						SetConfig: userID.String(),
					}

					if err := queries.SetUserContext(ctx, params); err != nil {
						return false, fmt.Errorf("failed to set user context: %w", err)
					}
				} else {
					logger.Debug("no user_id in context for PrepareConn")
				}

				// Extract user_id from claims and set it in PostgreSQL session for RLS
				claims, ok := organization.ClaimsFromContext(ctx)
				if ok {
					err := queries.SetUserContext(ctx, db.SetUserContextParams{
						SetConfig: claims.UserID.String(),
					})
					if err != nil {
						return false, fmt.Errorf("failed to set user context: %w", err)
					}
				}

				return true, nil
			}
			config.AfterRelease = func(c *pgx.Conn) bool {
				queries := db.New(c)

				if err := queries.ResetOrganizationContext(ctx); err != nil {
					logger.Warn("failed to reset organization context on connection release, destroying connection", "error", err)
					return false // Destroy connection to prevent data leakage
				}

				if err := queries.ResetUserContext(ctx); err != nil {
					logger.Warn("failed to reset user context on connection release, destroying connection", "error", err)
					return false // Destroy connection to prevent data leakage
				}

				if err := queries.ResetUserContext(ctx); err != nil {
					logger.Warn("failed to reset user context on connection release, destroying connection", "error", err)
					return false // Destroy connection to prevent user data leakage
				}

				return true // Keep connection in pool

			}
		},
	}

	db, err := psqldb.New(ctx, logger, cfg.Database, options...)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	defer db.Close()

	dbversion.MustAssertLatestVersion(ctx, logger, db.Pool)

	logger.Debug("database connected")

	server, err := organization.New(logger, &organization.Config{JWTSecret: []byte(cfg.JWTSecret)}, db)
	if err != nil {
		return fmt.Errorf("failed to create organization server: %w", err)
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
		server.AuthInterceptor(),
		validate.NewInterceptor(),
		loggingInterceptor,
	)

	orgPath, orgHandler := organizationv1connect.NewOrganizationServiceHandler(server, interceptors)
	mux.Handle(orgPath, orgHandler)

	clusterPath, clusterHandler := organizationv1connect.NewClusterServiceHandler(server, interceptors)
	mux.Handle(clusterPath, clusterHandler)

	pluginPath, pluginHandler := organizationv1connect.NewPluginServiceHandler(server, interceptors)
	mux.Handle(pluginPath, pluginHandler)

	// gRPC reflection for API discovery (used by Bruno, grpcurl, etc.)
	reflector := grpcreflect.NewStaticReflector(
		"organization.v1.OrganizationService",
		"organization.v1.ClusterService",
		"organization.v1.PluginService",
		"organization.v1.MemberService",
		"organization.v1.APIKeyService",
	)
	reflectPath, reflectHandler := grpcreflect.NewHandlerV1(reflector)
	mux.Handle(reflectPath, reflectHandler)
	reflectPathAlpha, reflectHandlerAlpha := grpcreflect.NewHandlerV1Alpha(reflector)
	mux.Handle(reflectPathAlpha, reflectHandlerAlpha)

	projectPath, projectHandler := organizationv1connect.NewProjectServiceHandler(server, interceptors)
	mux.Handle(projectPath, projectHandler)

	memberPath, memberHandler := organizationv1connect.NewMemberServiceHandler(server, interceptors)
	mux.Handle(memberPath, memberHandler)

	apiKeyPath, apiKeyHandler := organizationv1connect.NewAPIKeyServiceHandler(server, interceptors)
	mux.Handle(apiKeyPath, apiKeyHandler)

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "Connect-Protocol-Version"},
		AllowCredentials: true,
	})

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h2c.NewHandler(corsHandler.Handler(mux), &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("server listening", "addr", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}
