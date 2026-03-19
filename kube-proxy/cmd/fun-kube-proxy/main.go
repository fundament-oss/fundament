package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/kube-proxy/pkg/proxy"
)

type config struct {
	OpenFGA             authz.Config
	JWTSecret           string     `env:"JWT_SECRET,required,notEmpty"`
	ListenAddr          string     `env:"LISTEN_ADDR" envDefault:":8081"`
	LogLevel            slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins  []string   `env:"CORS_ALLOWED_ORIGINS"`
	KubeProxyMode       string     `env:"KUBE_PROXY_MODE" envDefault:"mock"`
	KubeProxyKubeconfig string     `env:"KUBE_PROXY_KUBECONFIG"`
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

	logger.Info("starting kube-proxy",
		"listen_addr", cfg.ListenAddr,
		"log_level", cfg.LogLevel.String(),
		"kube_proxy_mode", cfg.KubeProxyMode,
	)

	authzClient, err := authz.New(cfg.OpenFGA)
	if err != nil {
		return fmt.Errorf("failed to create OpenFGA client: %w", err)
	}

	server, err := proxy.New(logger, &proxy.Config{
		JWTSecret:           []byte(cfg.JWTSecret),
		CORSAllowedOrigins:  cfg.CORSAllowedOrigins,
		KubeProxyMode:       cfg.KubeProxyMode,
		KubeProxyKubeconfig: cfg.KubeProxyKubeconfig,
	}, authzClient)
	if err != nil {
		return fmt.Errorf("failed to create proxy server: %w", err)
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
