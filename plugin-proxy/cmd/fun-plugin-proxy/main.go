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

	"connectrpc.com/connect"
	"github.com/caarlos0/env/v11"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/installation"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
)

type config struct {
	// ListenAddr is the public surface. In this plan it serves only health
	// endpoints; the plugin asset/proxy routes land in Plan C.
	ListenAddr string `env:"LISTEN_ADDR" envDefault:":8082"`
	// InternalListenAddr carries the service-to-service PluginInstallationService
	// RPC. It is a separate listener so a NetworkPolicy can restrict it to
	// authn-api's ServiceAccount (FUN-17).
	InternalListenAddr string     `env:"INTERNAL_LISTEN_ADDR" envDefault:":8083"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	// Mode is "mock" or "real" and mirrors kube-api-proxy. Real-mode cluster
	// access lands in Plan C; this plan supports "mock" only.
	Mode string `env:"PLUGIN_PROXY_MODE" envDefault:"mock"`
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

	logger.Info("starting plugin-proxy",
		"listen_addr", cfg.ListenAddr,
		"internal_listen_addr", cfg.InternalListenAddr,
		"log_level", cfg.LogLevel.String(),
		"mode", cfg.Mode,
	)

	if cfg.Mode != "mock" {
		return fmt.Errorf("PLUGIN_PROXY_MODE=%q: real mode lands in Plan C, only %q is supported", cfg.Mode, "mock")
	}

	publicMux := http.NewServeMux()
	registerHealth(publicMux)

	internalMux := http.NewServeMux()
	registerHealth(internalMux)

	// PluginInstallationService is the internal RPC authn-api calls at mint
	// time. Real-mode cluster access lands in Plan C; this plan uses mocks.
	installSvc := installation.NewService(
		logger,
		installation.NewMockClusterClient(),
		installation.NewMockOrgIDResolver(),
	)
	installPath, installHandler := pluginproxyv1connect.NewPluginInstallationServiceHandler(
		installSvc,
		connect.WithInterceptors(connectrecovery.NewInterceptor(logger)),
	)
	internalMux.Handle(installPath, installHandler)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	publicSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h2c.NewHandler(publicMux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	internalSrv := &http.Server{
		Addr:              cfg.InternalListenAddr,
		Handler:           h2c.NewHandler(internalMux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 2)
	go func() {
		logger.Info("public surface listening", "addr", cfg.ListenAddr)
		if err := publicSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("public server: %w", err)
		}
	}()
	go func() {
		logger.Info("internal surface listening", "addr", cfg.InternalListenAddr)
		if err := internalSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("internal server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-serveErr:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = publicSrv.Shutdown(shutdownCtx)
	_ = internalSrv.Shutdown(shutdownCtx)
	return nil
}

func registerHealth(mux *http.ServeMux) {
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
