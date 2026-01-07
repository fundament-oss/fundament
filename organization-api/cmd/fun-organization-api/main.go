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
	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/fundament-oss/fundament/common/psqldb"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

type config struct {
	JWTSecret          string     `env:"JWT_SECRET,required,notEmpty" `
	DatabaseURL        string     `env:"DATABASE_URL,required,notEmpty"`
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

				// Extract tenant_id from context and set it in PostgreSQL session for RLS
				tenantID, ok := organization.TenantIDFromContext(ctx)
				if ok {
					if err := queries.SetTenantContext(ctx, tenantID.String()); err != nil {
						return false, err
					}
				}

				return true, nil
			}
			config.AfterRelease = func(c *pgx.Conn) bool {
				queries := db.New(c)

				if err := queries.ResetTenantContext(ctx); err != nil {
					logger.Warn("failed to reset tenant context on connection release, destroying connection", "error", err)
					return false // Destroy connection to prevent tenant data leakage
				}

				return true // Keep connection in pool

			}
		},
	}

	db, err := psqldb.New(ctx, logger, cfg.DatabaseURL, options...)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	defer db.Close()

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
		server.AuthInterceptor(), // First: authenticate and enrich context
		loggingInterceptor,       // Second: log with enriched context
	)

	orgPath, orgHandler := organizationv1connect.NewOrganizationServiceHandler(server, interceptors)
	mux.Handle(orgPath, orgHandler)

	clusterPath, clusterHandler := organizationv1connect.NewClusterServiceHandler(server, interceptors)
	mux.Handle(clusterPath, clusterHandler)

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
