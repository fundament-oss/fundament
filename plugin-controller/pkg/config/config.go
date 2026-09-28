package config

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	Namespace          string `env:"NAMESPACE,required,notEmpty"`
	FundamentClusterID string `env:"FUNDAMENT_CLUSTER_ID,required,notEmpty"`
	FundamentInstallID string `env:"FUNDAMENT_INSTALL_ID,required,notEmpty"`
	FundamentOrgID     string `env:"FUNDAMENT_ORGANIZATION_ID,required,notEmpty"`
	// CatalogAPIURL is the base URL of marketplace-catalog-api, which also
	// serves install.v1: the controller reads definitions there as this
	// cluster's workload (FUN-22).
	CatalogAPIURL string `env:"MARKETPLACE_CATALOG_API_URL,required,notEmpty"`
	// AuthnAPIURL is where the projected ServiceAccount token is exchanged for
	// a WorkloadToken bound to FundamentClusterID.
	AuthnAPIURL string `env:"FUNDAMENT_AUTHN_API_URL,required,notEmpty"`
	// TokenFile is the projected token's mount path (audience fundament-authn-api).
	TokenFile          string        `env:"FUNDAMENT_TOKEN_FILE" envDefault:"/var/run/secrets/fundament/token"`
	LogLevel           slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	HealthPort         int           `env:"HEALTH_PORT" envDefault:"8097"`
	StatusPollInterval time.Duration `env:"STATUS_POLL_INTERVAL" envDefault:"30s"`

	// AllowUnpinnedHash bypasses the definition-hash gate in
	// reconcilePluginScope: when true, a PluginInstallation with an empty
	// DefinitionHash is accepted and the definition is fetched from the
	// catalog; the hash is computed on receipt but not compared.
	// Intended for local development (`just dev` / mock clusters). Never
	// enable in production — see charts/fundament/templates/plugin-controller.yaml
	// for the escalation implications.
	AllowUnpinnedHash bool `env:"PLUGIN_CONTROLLER_ALLOW_UNPINNED_HASH" envDefault:"false"`
}

// Validate checks what env parsing cannot. The cluster and organization ids
// must be tenant.clusters / tenant.organizations UUIDs: authn-api rejects
// anything else, which would otherwise only surface as every definition
// fetch failing at runtime.
func (c *Config) Validate() error {
	if _, err := uuid.Parse(c.FundamentClusterID); err != nil {
		return fmt.Errorf("FUNDAMENT_CLUSTER_ID %q is not a cluster UUID: %w", c.FundamentClusterID, err)
	}
	if _, err := uuid.Parse(c.FundamentOrgID); err != nil {
		return fmt.Errorf("FUNDAMENT_ORGANIZATION_ID %q is not an organization UUID: %w", c.FundamentOrgID, err)
	}
	return nil
}
