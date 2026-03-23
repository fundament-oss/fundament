package config

import (
	"log/slog"
	"time"
)

type Config struct {
	Namespace          string        `env:"NAMESPACE,required,notEmpty"`
	FundamentClusterID string        `env:"FUNDAMENT_CLUSTER_ID,required,notEmpty"`
	FundamentInstallID string        `env:"FUNDAMENT_INSTALL_ID,required,notEmpty"`
	FundamentOrgID     string        `env:"FUNDAMENT_ORGANIZATION_ID,required,notEmpty"`
	LogLevel           slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	HealthPort         int           `env:"HEALTH_PORT" envDefault:"8097"`
	StatusPollInterval time.Duration `env:"STATUS_POLL_INTERVAL" envDefault:"30s"`
}
