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
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/dcim-authn-api/pkg/dcimauthn"
)

type config struct {
	JWTSecret          string        `env:"JWT_SECRET,required,notEmpty"`
	OIDCIssuer         string        `env:"OIDC_ISSUER,required,notEmpty" envDefault:"https://dex-dcim.fundament.localhost:8443"`
	OIDCDiscoveryURL   string        `env:"OIDC_DISCOVERY_URL"`
	ClientID           string        `env:"OIDC_CLIENT_ID,required,notEmpty" envDefault:"dcim"`
	RedirectURL        string        `env:"OIDC_REDIRECT_URL,required,notEmpty" envDefault:"https://dcim-authn.fundament.localhost:8443/callback"`
	FrontendURL        string        `env:"FRONTEND_URL,required,notEmpty" envDefault:"https://dcim.fundament.localhost:8443"`
	CookieDomain       string        `env:"COOKIE_DOMAIN,required,notEmpty" envDefault:"fundament.localhost"`
	CookieSecure       bool          `env:"COOKIE_SECURE"`
	ListenAddr         string        `env:"LISTEN_ADDR" envDefault:":8080"`
	TokenExpiry        time.Duration `env:"TOKEN_EXPIRY" envDefault:"24h"`
	MaxSessionAge      time.Duration `env:"MAX_SESSION_AGE" envDefault:"168h"`
	LogLevel           slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins []string      `env:"CORS_ALLOWED_ORIGINS" envDefault:"https://dcim.fundament.localhost:8443"`
	// PasswordLoginEnabled exposes the OAuth2 resource-owner-password-credentials
	// (ROPC) login endpoint. ROPC is convenient against dex's static passwords for
	// local/dev, but is discouraged for real identity providers, so it defaults to
	// disabled and must be explicitly enabled (e.g. only in local/dev) in favour of
	// the redirect-based OIDC flow.
	PasswordLoginEnabled bool `env:"PASSWORD_LOGIN_ENABLED" envDefault:"false"`
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

	logger.Info("starting dcim-authn-api",
		"listen_addr", cfg.ListenAddr,
		"oidc_issuer", cfg.OIDCIssuer,
	)

	ctx := context.Background()

	// Use OIDCIssuer as discovery URL if not explicitly set.
	if cfg.OIDCDiscoveryURL == "" {
		cfg.OIDCDiscoveryURL = cfg.OIDCIssuer
	}

	// When running in k8s the issuer (external URL) is unreachable from within the cluster,
	// but the internal discovery URL is. Use insecure issuer context to allow the mismatch.
	if cfg.OIDCDiscoveryURL != cfg.OIDCIssuer {
		ctx = oidc.InsecureIssuerURLContext(ctx, cfg.OIDCIssuer)
	}

	provider, err := oidc.NewProvider(ctx, cfg.OIDCDiscoveryURL)
	if err != nil {
		return fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	endpoint := provider.Endpoint()

	var verifier *oidc.IDTokenVerifier
	if cfg.OIDCDiscoveryURL != cfg.OIDCIssuer {
		endpoint.TokenURL = strings.Replace(endpoint.TokenURL, cfg.OIDCIssuer, cfg.OIDCDiscoveryURL, 1)
		jwksURL := cfg.OIDCDiscoveryURL + "/keys"
		keySet := oidc.NewRemoteKeySet(ctx, jwksURL)
		verifier = oidc.NewVerifier(cfg.OIDCIssuer, keySet, &oidc.Config{ClientID: cfg.ClientID})
	} else {
		verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	}

	oauth2Config := &oauth2.Config{
		ClientID:    cfg.ClientID,
		RedirectURL: cfg.RedirectURL,
		Endpoint:    endpoint,
		Scopes:      []string{oidc.ScopeOpenID, "profile", "email"},
	}

	sessionStore, err := dcimauthn.NewSessionStore([]byte(cfg.JWTSecret))
	if err != nil {
		return fmt.Errorf("creating session store: %w", err)
	}
	sessionStore.ConfigureOptions(cfg.CookieDomain, cfg.CookieSecure)

	serverCfg := &dcimauthn.Config{
		TokenExpiry:   cfg.TokenExpiry,
		JWTSecret:     []byte(cfg.JWTSecret),
		CookieDomain:  cfg.CookieDomain,
		CookieSecure:  cfg.CookieSecure,
		FrontendURL:   cfg.FrontendURL,
		MaxSessionAge: cfg.MaxSessionAge,
	}

	server := dcimauthn.New(logger, serverCfg, oauth2Config, verifier, sessionStore)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if cfg.PasswordLoginEnabled {
		mux.HandleFunc("POST /login/password", server.HandlePasswordLogin)
	} else {
		logger.Info("password login (ROPC) endpoint disabled")
	}
	mux.HandleFunc("GET /login", server.HandleLogin)
	mux.HandleFunc("GET /callback", server.HandleCallback)
	mux.HandleFunc("POST /refresh", server.HandleRefresh)
	mux.HandleFunc("POST /logout", server.HandleLogout)
	mux.HandleFunc("GET /userinfo", server.HandleUserInfo)

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
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
