package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/authn-api/config"
	"github.com/fundament-oss/fundament/authn-api/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/authn-api/sqlc/db"
)

type AuthnServer struct {
	config       *config.Config
	oauth2Config *oauth2.Config
	oidcVerifier *oidc.IDTokenVerifier
	queries      *db.Queries
	sessionStore *SessionStore
	logger       *slog.Logger
}

func main() {
	cfg := config.Load()

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
		logger.Error("failed to create OIDC provider", "error", err)
		os.Exit(1)
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
	storage, err := NewStorage(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	logger.Debug("database connected")

	// Create session store for OAuth state management
	sessionStore := NewSessionStore(cfg.JWTSecret)
	sessionStore.ConfigureOptions(cfg.CookieDomain, cfg.CookieSecure)

	server := &AuthnServer{
		config:       cfg,
		oauth2Config: oauth2Config,
		oidcVerifier: verifier,
		queries:      storage.Queries,
		sessionStore: sessionStore,
		logger:       logger,
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

	// HTTP endpoints for authentication flow
	mux.HandleFunc("/login", server.handleLogin)
	mux.HandleFunc("/login/password", server.handlePasswordLogin)
	mux.HandleFunc("/callback", server.handleCallback)
	mux.HandleFunc("/refresh", server.handleRefresh)
	mux.HandleFunc("/logout", server.handleLogout)

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
		logger.Error("server failed", "error", err)
		storage.Close()
		os.Exit(1)
	}
}
