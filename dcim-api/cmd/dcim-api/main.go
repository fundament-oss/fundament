package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/dcim-api/pkg/dcim"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type config struct {
	Database           psqldb.Config
	ListenAddr         string `env:"LISTEN_ADDR" envDefault:":8080"`
	LogLevel           string `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins string `env:"CORS_ALLOWED_ORIGINS"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	level := slog.LevelInfo
	_ = level.UnmarshalText([]byte(cfg.LogLevel))
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	ctx := context.Background()

	database, err := psqldb.New(ctx, logger, cfg.Database)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer database.Close()

	server := dcim.New(logger, database)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", server.Handler())

	allowedOrigins := strings.Split(cfg.CORSAllowedOrigins, ",")
	c := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Authorization", "Content-Type", "Connect-Protocol-Version", "Connect-Timeout-Ms", "Grpc-Timeout", "X-Grpc-Web", "X-User-Agent"},
		ExposedHeaders: []string{"Grpc-Status", "Grpc-Message", "Grpc-Status-Details-Bin"},
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h2c.NewHandler(c.Handler(mux), &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("starting api", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	return nil
}
