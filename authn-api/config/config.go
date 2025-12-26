package config

import (
	"log"
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	JWTSecret          []byte
	OIDCIssuer         string
	OIDCDiscoveryURL   string // URL to fetch OIDC discovery document (defaults to OIDCIssuer)
	ClientID           string
	RedirectURL        string
	FrontendURL        string
	CookieDomain       string
	CookieSecure       bool
	DatabaseURL        string
	ListenAddr         string
	TokenExpiry        time.Duration
	LogLevel           slog.Level
	CORSAllowedOrigins []string
}

func Load() *Config {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	oidcIssuer := getEnv("OIDC_ISSUER", "http://localhost:5556")
	return &Config{
		OIDCIssuer:         oidcIssuer,
		OIDCDiscoveryURL:   getEnv("OIDC_DISCOVERY_URL", oidcIssuer),
		ClientID:           getEnv("OIDC_CLIENT_ID", "authn-api"),
		RedirectURL:        getEnv("OIDC_REDIRECT_URL", "http://localhost:10100/callback"),
		FrontendURL:        getEnv("FRONTEND_URL", "http://localhost:5173"),
		CookieDomain:       getEnv("COOKIE_DOMAIN", "localhost"),
		CookieSecure:       getEnv("COOKIE_SECURE", "false") == "true",
		JWTSecret:          []byte(jwtSecret),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://authn_api:password@localhost:5432/fundament"),
		ListenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		TokenExpiry:        24 * time.Hour,
		LogLevel:           parseLogLevel(getEnv("LOG_LEVEL", "info")),
		CORSAllowedOrigins: parseCORSOrigins(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:4200")),
	}
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseCORSOrigins(origins string) []string {
	if origins == "" {
		return []string{}
	}
	parts := strings.Split(origins, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
