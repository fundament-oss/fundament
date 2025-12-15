package config

import (
	"log"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	JWTSecret          []byte
	DatabaseURL        string
	ListenAddr         string
	LogLevel           slog.Level
	CORSAllowedOrigins []string
}

func Load() *Config {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	return &Config{
		JWTSecret:          []byte(jwtSecret),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://organization_api:password@localhost:5432/fundament"),
		ListenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		LogLevel:           parseLogLevel(getEnv("LOG_LEVEL", "info")),
		CORSAllowedOrigins: parseCORSOrigins(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
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
