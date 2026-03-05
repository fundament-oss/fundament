package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/fundament-oss/fundament/organization-api/pkg/clock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
	dbgen "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

type config struct {
	Database           psqldb.Config
	OpenFGA            authz.Config
	JWTSecret          string     `env:"JWT_SECRET,required,notEmpty" `
	ListenAddr         string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins []string   `env:"CORS_ALLOWED_ORIGINS"`
	PrometheusMetalURL string     `env:"PROMETHEUS_METAL_URL"`
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
				queries := dbgen.New(conn)

				// Extract organization_id from context and set it in PostgreSQL session for RLS
				if organizationID, ok := organization.OrganizationIDFromContext(ctx); ok {
					logger.Debug("setting organization context for RLS", "organization_id", organizationID.String())

					params := dbgen.SetOrganizationContextParams{
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

					params := dbgen.SetUserContextParams{
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
					err := queries.SetUserContext(ctx, dbgen.SetUserContextParams{
						SetConfig: claims.UserID.String(),
					})
					if err != nil {
						return false, fmt.Errorf("failed to set user context: %w", err)
					}
				}

				return true, nil
			}
			config.AfterRelease = func(c *pgx.Conn) bool {
				queries := dbgen.New(c)

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
		return fmt.Errorf("failed to setup to database: %w", err)
	}

	defer db.Close()

	dbversion.MustAssertLatestVersion(ctx, logger, db.Pool)

	logger.Debug("database connected")

	logger.Debug("connecting to OpenFGA",
		"api_url", cfg.OpenFGA.APIURL,
		"store_id", cfg.OpenFGA.StoreID,
	)

	authzClient, err := authz.New(cfg.OpenFGA)
	if err != nil {
		return fmt.Errorf("failed to create OpenFGA client: %w", err)
	}

	logger.Debug("OpenFGA client connected")

	mockClient := prom.NewMockClient(func(ctx context.Context) ([]prom.ClusterInfo, error) {
		q := dbgen.New(db.Pool)
		rows, err := q.ClusterList(ctx)
		if err != nil {
			return nil, err
		}
		clusters := make([]prom.ClusterInfo, 0, len(rows))
		for _, row := range rows {
			pools, err := q.NodePoolListByClusterID(ctx, dbgen.NodePoolListByClusterIDParams{ClusterID: row.ID})
			if err != nil {
				return nil, err
			}
			nodePools := make([]prom.NodePoolInfo, 0, len(pools))
			for _, p := range pools {
				nodePools = append(nodePools, prom.NodePoolInfo{
					Name:         p.Name,
					MachineType:  p.MachineType,
					AutoscaleMin: p.AutoscaleMin,
					AutoscaleMax: p.AutoscaleMax,
				})
			}
			clusters = append(clusters, prom.ClusterInfo{
				ID:        row.ID.String(),
				Name:      row.Name,
				NodePools: nodePools,
			})
		}
		return clusters, nil
	})

	metalPromClient := promClient("metal-stack", cfg.PrometheusMetalURL, mockClient, logger)

	server, err := organization.New(logger, &organization.Config{
		JWTSecret:             []byte(cfg.JWTSecret),
		CORSAllowedOrigins:    cfg.CORSAllowedOrigins,
		Clock:                 clock.New(),
		MockPrometheusClient:  mockClient,
		MetalPrometheusClient: metalPromClient,
	}, db, authzClient)
	if err != nil {
		return fmt.Errorf("failed to create organization server: %w", err)
	}

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h2c.NewHandler(server.Handler(), &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("server listening", "addr", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}

// promClient selects the appropriate Prometheus client based on the URL value:
//   - ""     → StubClient (returns empty data, no metrics configured)
//   - "mock" → MockClient (in-process generated data, no real Prometheus needed)
//   - other  → HTTPClient targeting the given URL
func promClient(name, url string, mock *prom.MockClient, logger *slog.Logger) prom.Client {
	switch url {
	case "":
		logger.Info("Prometheus not configured, metrics will return empty data", "client", name)
		return prom.StubClient{}
	case "mock":
		logger.Info("Prometheus mock mode enabled", "client", name)
		return mock
	default:
		logger.Info("Prometheus configured", "client", name, "url", url)
		return prom.NewHTTPClient(url)
	}
}
