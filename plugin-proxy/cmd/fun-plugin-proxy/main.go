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
	"connectrpc.com/validate"
	"github.com/caarlos0/env/v11"
	"github.com/svrana/go-connect-middleware/interceptors/logging"

	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/proto/gen/plugin_proxy/v1/pluginproxyv1connect"
	"github.com/fundament-oss/fundament/plugin-proxy/pkg/service"
)

type config struct {
	// ListenAddr serves health endpoints today; plugin asset/proxy routes
	// land here in future work.
	ListenAddr string `env:"LISTEN_ADDR" envDefault:":8080"`
	// InternalListenAddr carries the service-to-service PluginInstallationService
	// RPC. Separate listener so a NetworkPolicy can restrict it to authn-api.
	InternalListenAddr string     `env:"INTERNAL_LISTEN_ADDR" envDefault:":8081"`
	LogLevel           slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	// Mode is "mock" or "real" and mirrors kube-api-proxy. Only "mock" is
	// supported today; real-mode cluster access is future work.
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
		return fmt.Errorf("PLUGIN_PROXY_MODE=%q: only %q is supported", cfg.Mode, "mock")
	}

	publicMux := http.NewServeMux()
	registerHealth(publicMux)

	internalMux := http.NewServeMux()
	registerHealth(internalMux)

	s := service.New(logger, service.NewMockClusterAccess())

	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)

	interceptors := connect.WithInterceptors(
		connectrecovery.NewInterceptor(logger),
		validate.NewInterceptor(),
		loggingInterceptor,
	)

	internalMux.Handle(pluginproxyv1connect.NewPluginInstallationServiceHandler(s, interceptors))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	publicSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           publicMux,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
	internalSrv := &http.Server{
		Addr:              cfg.InternalListenAddr,
		Handler:           internalMux,
		Protocols:         protocols,
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

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-serveErr:
		logger.Error("server error, shutting down", "error", err)
		runErr = err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := publicSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("public server shutdown", "error", err)
	}
	if err := internalSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("internal server shutdown", "error", err)
	}
	return runErr
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
