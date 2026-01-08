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

	"connectrpc.com/connect"
	"github.com/caarlos0/env/v11"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/authn-api/pkg/authn"
	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/common/psqldb"
)

type config struct {
	Database           psqldb.Config
	JWTSecret          string        `env:"JWT_SECRET,required,notEmpty" `
	OIDCIssuer         string        `env:"OIDC_ISSUER,required,notEmpty" envDefault:"http://localhost:5556"`
	OIDCDiscoveryURL   string        `env:"OIDC_DISCOVERY_URL"` // URL to fetch OIDC discovery document (defaults to OIDCIssuer)
	ClientID           string        `env:"OIDC_CLIENT_ID,required,notEmpty" envDefault:"authn-api"`
	RedirectURL        string        `env:"OIDC_REDIRECT_URL,required,notEmpty" envDefault:"http://authn.127.0.0.1.nip.io:8080/callback"`
	FrontendURL        string        `env:"FRONTEND_URL,required,notEmpty" envDefault:"http://console.127.0.0.1.nip.io:8080"`
	CookieDomain       string        `env:"COOKIE_DOMAIN,required,notEmpty" envDefault:"localhost"`
	CookieSecure       bool          `env:"COOKIE_SECURE,required,notEmpty"`
	DatabaseURL        string        `env:"DATABASE_URL,required,notEmpty"`
	ListenAddr         string        `env:"LISTEN_ADDR" envDefault:":8080"`
	TokenExpiry        time.Duration `env:"TOKEN_EXPIRY" envDefault:"24h"`
	LogLevel           slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins []string      `env:"CORS_ALLOWED_ORIGINS" envDefault:"http://localhost:5173,http://localhost:4200,http://console.127.0.0.1.nip.io:8080"`
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

	logger.Info("starting authn-api",
		"listen_addr", cfg.ListenAddr,
		"oidc_issuer", cfg.OIDCIssuer,
		"log_level", cfg.LogLevel.String(),
	)

	ctx := context.Background()

	logger.Debug("connecting to OIDC provider", "issuer", cfg.OIDCIssuer, "discovery_url", cfg.OIDCDiscoveryURL)

	// Use internal URL for discovery
	if cfg.OIDCDiscoveryURL != cfg.OIDCIssuer {
		ctx = oidc.InsecureIssuerURLContext(ctx, cfg.OIDCIssuer)
	}

	provider, err := oidc.NewProvider(ctx, cfg.OIDCDiscoveryURL)
	if err != nil {
		return fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	logger.Debug("OIDC provider connected")

	endpoint := provider.Endpoint()

	// Override the token URL to use internal discovery URL instead of issuer
	// When running in k8s, the issuer (external URL) is unreachable from within the cluster
	// but the discovery URL (internal service) is. We need to rewrite all endpoints.
	var verifier *oidc.IDTokenVerifier
	if cfg.OIDCDiscoveryURL != cfg.OIDCIssuer {
		// Replace the issuer domain in endpoints with discovery domain
		endpoint.TokenURL = strings.Replace(endpoint.TokenURL, cfg.OIDCIssuer, cfg.OIDCDiscoveryURL, 1)
		// Keep AuthURL pointing to external issuer for browser redirects
		// (the provider already returns the issuer-based URL)

		// Create a custom keyset that fetches JWKS from the internal discovery URL
		// The provider's verifier would try to fetch from the issuer URL which is unreachable
		jwksURL := cfg.OIDCDiscoveryURL + "/keys"
		logger.Debug("using custom JWKS URL", "jwks_url", jwksURL)
		keySet := oidc.NewRemoteKeySet(ctx, jwksURL)
		verifier = oidc.NewVerifier(cfg.OIDCIssuer, keySet, &oidc.Config{ClientID: cfg.ClientID})
	} else {
		verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	}

	oauth2Config := &oauth2.Config{
		ClientID:    cfg.ClientID,
		RedirectURL: cfg.RedirectURL,
		Endpoint:    endpoint,
		Scopes:      []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}

	logger.Debug("connecting to database")
	db, err := psqldb.New(ctx, logger, cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	defer db.Close()

	logger.Debug("database connected")

	// Create session store for OAuth state management
	sessionStore := authn.NewSessionStore([]byte(cfg.JWTSecret))
	sessionStore.ConfigureOptions(cfg.CookieDomain, cfg.CookieSecure)

	authnCfg := &authn.Config{
		TokenExpiry:  cfg.TokenExpiry,
		JWTSecret:    []byte(cfg.JWTSecret),
		CookieDomain: cfg.CookieDomain,
		CookieSecure: cfg.CookieSecure,
		FrontendURL:  cfg.FrontendURL,
	}

	server, err := authn.New(logger, authnCfg, oauth2Config, verifier, sessionStore, db)
	if err != nil {
		return fmt.Errorf("failed to create authn api: %w", err)
	}

	mux := http.NewServeMux()

	// Connect RPC handler for GetUserInfo
	loggingInterceptor := logging.UnaryServerInterceptor(
		logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
			logger.Log(ctx, slog.Level(level), msg, fields...)
		}),
		logging.WithLogOnEvents(logging.FinishCall),
	)
	path, handler := authnv1connect.NewAuthnServiceHandler(server, connect.WithInterceptors(loggingInterceptor))
	mux.Handle(path, handler)

	// HTTP endpoints for authentication flow (registers routes on mux)
	_ = authnhttp.HandlerFromMux(server, mux)

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
