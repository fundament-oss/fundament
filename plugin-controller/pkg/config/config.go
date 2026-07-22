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

	// AllowUnpinnedHash bypasses the definition-hash gate in
	// reconcilePluginScope: when true, a PluginInstallation with an empty
	// DefinitionHash is accepted and whatever the plugin returns from
	// GetDefinition is materialised as the scope ClusterRole. Intended for
	// local development (`just dev` / mock clusters). Never enable in
	// production — see charts/fundament/templates/plugin-controller.yaml for
	// the escalation implications.
	AllowUnpinnedHash bool `env:"PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH" envDefault:"false"`
}
