package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/fundament-oss/fundament/organization-api/pkg/clock"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/idempotency"
	"github.com/fundament-oss/fundament/common/psqldb"
	dbgen "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

type config struct {
	Database             psqldb.Config
	OpenFGA              authz.Config
	JWTSecret            string     `env:"JWT_SECRET,required,notEmpty" `
	ListenAddr           string     `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel             slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins   []string   `env:"CORS_ALLOWED_ORIGINS"`
	PrometheusURL        string     `env:"PROMETHEUS_URL" envDefault:"mock"`
	KubeAPIProxyURL      string     `env:"KUBE_API_PROXY_URL"`
	KubeAPIProxyInsecure bool       `env:"KUBE_API_PROXY_INSECURE" envDefault:"false"`
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

	db, err := organization.NewDB(ctx, logger, cfg.Database)
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

	idempotencyStore := idempotency.NewStore(db.Pool, idempotency.Config{}, logger)
	go idempotencyStore.StartCleanup(ctx)

	server, err := organization.New(logger, &organization.Config{
		JWTSecret:            []byte(cfg.JWTSecret),
		CORSAllowedOrigins:   cfg.CORSAllowedOrigins,
		Clock:                clock.New(),
		MockPrometheusClient: mockClient,
		PrometheusURL:        cfg.PrometheusURL,
		KubeAPIProxyURL:      cfg.KubeAPIProxyURL,
		KubeAPIProxyInsecure: cfg.KubeAPIProxyInsecure,
	}, db, authzClient, idempotencyStore)
	if err != nil {
		return fmt.Errorf("failed to create organization server: %w", err)
	}

	// Health endpoints are registered on an outer mux so they bypass CORS
	// and Connect interceptors.
	outerMux := http.NewServeMux()
	outerMux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	outerMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var errs []string
		if err := db.Pool.Ping(ctx); err != nil {
			errs = append(errs, "database: "+err.Error())
		}
		if err := authzClient.Healthy(ctx); err != nil {
			errs = append(errs, "openfga: "+err.Error())
		}

		if len(errs) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(strings.Join(errs, "\n")))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	outerMux.Handle("/", server.Handler())

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h2c.NewHandler(outerMux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("server listening", "addr", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}
