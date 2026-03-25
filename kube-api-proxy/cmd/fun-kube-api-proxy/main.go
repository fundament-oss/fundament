package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/proxy"
)

type config struct {
	OpenFGA            authz.Config
	JWTSecret          string     `env:"JWT_SECRET,required,notEmpty"`
	ListenAddr         string     `env:"LISTEN_ADDR" envDefault:":8081"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins []string   `env:"CORS_ALLOWED_ORIGINS"`
	KubeProxyMode      string `env:"KUBE_API_PROXY_MODE" envDefault:"mock"`
	GardenerKubeconfig string `env:"GARDENER_KUBECONFIG"` // required when Mode == "real"
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

	logger.Info("starting kube-api-proxy",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
		"mode", cfg.KubeProxyMode,
	)

	authzClient, err := authz.New(cfg.OpenFGA)
	if err != nil {
		return fmt.Errorf("failed to create OpenFGA client: %w", err)
	}

	var gardenerClient *gardener.Client
	if cfg.KubeProxyMode == "real" {
		if cfg.GardenerKubeconfig == "" {
			return fmt.Errorf("GARDENER_KUBECONFIG required for real mode")
		}
		var err error
		gardenerClient, err = gardener.New(cfg.GardenerKubeconfig, logger)
		if err != nil {
			return fmt.Errorf("create gardener client: %w", err)
		}
	}

	server, err := proxy.New(logger, &proxy.Config{
		JWTSecret:          []byte(cfg.JWTSecret),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		Mode:               cfg.KubeProxyMode,
		GardenerClient:     gardenerClient,
	}, authzClient)
	if err != nil {
		return fmt.Errorf("failed to create proxy server: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h2c.NewHandler(server.Handler(), &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
	}()

	logger.Info("server listening", "addr", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}
