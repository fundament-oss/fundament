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
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"
	"github.com/caarlos0/env/v11"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/rs/cors"
	"github.com/svrana/go-connect-middleware/interceptors/logging"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/oauth2"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/authn-api/pkg/authn"
	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/connectrecovery"
	"github.com/fundament-oss/fundament/common/dbversion"
	"github.com/fundament-oss/fundament/common/psqldb"
)

type config struct {
	Database           psqldb.Config
	OpenFGA            authz.Config
	JWTSecret          string        `env:"JWT_SECRET,required,notEmpty" `
	OIDCIssuer         string        `env:"OIDC_ISSUER,required,notEmpty" envDefault:"http://localhost:5556"`
	OIDCDiscoveryURL   string        `env:"OIDC_DISCOVERY_URL"` // URL to fetch OIDC discovery document (defaults to OIDCIssuer)
	ClientID           string        `env:"OIDC_CLIENT_ID,required,notEmpty" envDefault:"authn-api"`
	RedirectURL        string        `env:"OIDC_REDIRECT_URL,required,notEmpty" envDefault:"http://authn.fundament.localhost:8080/callback"`
	FrontendURL        string        `env:"FRONTEND_URL,required,notEmpty" envDefault:"http://console.fundament.localhost:8080"`
	CookieDomain       string        `env:"COOKIE_DOMAIN,required,notEmpty" envDefault:"fundament.localhost"`
	CookieSecure       bool          `env:"COOKIE_SECURE,required,notEmpty"`
	DatabaseURL        string        `env:"DATABASE_URL,required,notEmpty"`
	ListenAddr         string        `env:"LISTEN_ADDR" envDefault:":8080"`
	TokenExpiry        time.Duration `env:"TOKEN_EXPIRY" envDefault:"24h"`
	LogLevel           slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	CORSAllowedOrigins []string      `env:"CORS_ALLOWED_ORIGINS" envDefault:"http://localhost:5173,http://localhost:4200,http://console.fundament.localhost:8080"`

	GardenerMode       string `env:"GARDENER_MODE" envDefault:"mock"` // "real" or "mock" (disabled)
	GardenerKubeconfig string `env:"GARDENER_KUBECONFIG"`             // path to Gardener kubeconfig
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

	// Create session store for OAuth state management
	sessionStore := authn.NewSessionStore([]byte(cfg.JWTSecret))
	sessionStore.ConfigureOptions(cfg.CookieDomain, cfg.CookieSecure)

	var gardenerClient authn.GardenerClient
	switch cfg.GardenerMode {
	case "real":
		if cfg.GardenerKubeconfig == "" {
			return fmt.Errorf("GARDENER_KUBECONFIG required when GARDENER_MODE=real")
		}
		providerCfg := gardener.NewProviderConfig()
		realClient, err := gardener.NewReal(cfg.GardenerKubeconfig, providerCfg, logger)
		if err != nil {
			return fmt.Errorf("create gardener client: %w", err)
		}
		gardenerClient = &gardenerAdapter{client: realClient}
		logger.Info("gardener client enabled", "mode", cfg.GardenerMode)
	case "mock":
		logger.Info("gardener client disabled (cluster token endpoint unavailable)")
	default:
		return fmt.Errorf("invalid GARDENER_MODE: %s (must be 'real' or 'mock')", cfg.GardenerMode)
	}

	authnCfg := &authn.Config{
		TokenExpiry:  cfg.TokenExpiry,
		JWTSecret:    []byte(cfg.JWTSecret),
		CookieDomain: cfg.CookieDomain,
		CookieSecure: cfg.CookieSecure,
		FrontendURL:  cfg.FrontendURL,
	}

	server, err := authn.New(logger, authnCfg, oauth2Config, verifier, sessionStore, db, gardenerClient, authzClient)
	if err != nil {
		return fmt.Errorf("failed to create authn api: %w", err)
	}

	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var errs []string

		if err := db.Pool.Ping(ctx); err != nil {
			errs = append(errs, "database: "+err.Error())
		}
		if err := authzClient.Healthy(ctx); err != nil {
			errs = append(errs, "openfga: "+err.Error())
		}
		if err := checkOIDC(ctx, cfg.OIDCDiscoveryURL); err != nil {
			errs = append(errs, "oidc: "+err.Error())
		}

		if len(errs) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(strings.Join(errs, "\n")))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Connect RPC handler for GetUserInfo
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

	path, handler := authnv1connect.NewAuthnServiceHandler(server, interceptors)
	mux.Handle(path, handler)

	tokenPath, tokenHandler := authnv1connect.NewTokenServiceHandler(server, interceptors)
	mux.Handle(tokenPath, tokenHandler)

	// gRPC reflection for API discovery (used by Bruno, grpcurl, etc.)
	reflector := grpcreflect.NewStaticReflector(
		"authn.v1.AuthnService",
		"authn.v1.TokenService",
	)
	reflectPath, reflectHandler := grpcreflect.NewHandlerV1(reflector)
	mux.Handle(reflectPath, reflectHandler)
	reflectPathAlpha, reflectHandlerAlpha := grpcreflect.NewHandlerV1Alpha(reflector)
	mux.Handle(reflectPathAlpha, reflectHandlerAlpha)

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

func checkOIDC(ctx context.Context, discoveryURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL+"/.well-known/openid-configuration", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// gardenerAdapter adapts the cluster-worker Gardener client to authn-api's GardenerClient interface.
type gardenerAdapter struct {
	client *gardener.RealClient
}

func (a *gardenerAdapter) RequestAdminKubeconfig(ctx context.Context, clusterID uuid.UUID, expirationSeconds int64) (*authn.AdminKubeconfig, error) {
	kc, err := a.client.RequestAdminKubeconfig(ctx, clusterID, expirationSeconds)
	if err != nil {
		return nil, fmt.Errorf("request admin kubeconfig: %w", err)
	}
	return &authn.AdminKubeconfig{
		Kubeconfig: kc.Kubeconfig,
		ExpiresAt:  kc.ExpiresAt,
	}, nil
}
