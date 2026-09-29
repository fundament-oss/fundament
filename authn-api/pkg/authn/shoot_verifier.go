package authn

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/fundament-oss/fundament/authn-api/pkg/shootverify"
	"github.com/fundament-oss/fundament/common/gardener"
)

// Seeded local-development identities (db/testdata/001_0101-content.sql):
// the acme-corp cluster every non-Gardener verifier answers for.
const (
	SeededClusterID      = "019b4000-2000-7000-8000-000000000001"
	SeededOrganizationID = "019b4000-0000-7000-8000-000000000001"
)

// ShootVerifierConfig selects how authn-api verifies shoot workload tokens
// (FUN-22). Modes, in order of precedence:
//
//   - Mode "mock": an HMAC stand-in for tests and mock-mode environments.
//   - GardenerMode "real": TokenReview on the shoot resolved via Gardener.
//   - a sandbox kubeconfig on disk: TokenReview on the local plugin sandbox.
//   - otherwise: every exchange is denied; no cluster hosts a controller.
//
// Mock and sandbox accept only LocalClusterID and report LocalOrganizationID;
// Gardener reads both from the shoot.
type ShootVerifierConfig struct {
	Mode                    string
	GardenerMode            string
	GardenerKubeconfig      string
	PluginSandboxKubeconfig string
	MockSecret              string
	// AllowDefaultMockSecret accepts DefaultMockShootSecret in mock mode.
	// Only for environments whose seeded logins are public (previews): a
	// forged token then reads nothing those logins cannot.
	AllowDefaultMockSecret bool
	LocalClusterID         string
	LocalOrganizationID    string
}

// DefaultMockShootSecret is the binary's MOCK_SHOOT_SECRET default. It is
// public, so mock mode refuses it unless AllowDefaultMockSecret is set.
const DefaultMockShootSecret = "mock-shoot-secret" //nolint:gosec // a public test key, refused by default

// validate rejects mode strings that would otherwise silently select a
// different verifier, and the public mock secret where it was not opted into.
func (cfg *ShootVerifierConfig) validate() error {
	switch cfg.Mode {
	case "auto", "mock":
	default:
		return fmt.Errorf("invalid SHOOT_VERIFIER_MODE %q: want auto or mock", cfg.Mode)
	}
	switch cfg.GardenerMode {
	case "mock", "real":
	default:
		return fmt.Errorf("invalid GARDENER_MODE %q: want mock or real", cfg.GardenerMode)
	}
	if cfg.Mode != "mock" {
		return nil
	}
	if cfg.MockSecret == "" {
		return fmt.Errorf("SHOOT_VERIFIER_MODE=mock needs a MOCK_SHOOT_SECRET")
	}
	if cfg.MockSecret == DefaultMockShootSecret && !cfg.AllowDefaultMockSecret {
		return fmt.Errorf("SHOOT_VERIFIER_MODE=mock with the public default MOCK_SHOOT_SECRET: " +
			"set a secret, or MOCK_SHOOT_SECRET_ALLOW_DEFAULT=true where the seeded logins are public too")
	}
	return nil
}

// UsesGardener reports whether tokens are verified against real shoots. The
// sandbox's plugin-controller only exists in the other modes, so only they
// allow-list its ServiceAccount next to the fixed shoot subject.
func (cfg *ShootVerifierConfig) UsesGardener() bool {
	return cfg.Mode != "mock" && cfg.GardenerMode == "real"
}

// NewShootVerifier builds the guarded verifier for cfg. It never fails on a
// missing Gardener or sandbox: verification then denies per exchange while
// the pod stays ready, so a control plane without shoot access still serves
// every other endpoint.
func NewShootVerifier(logger *slog.Logger, cfg *ShootVerifierConfig) (shootverify.Verifier, error) {
	inner, err := newShootVerifier(logger, cfg)
	if err != nil {
		return nil, err
	}
	return shootverify.NewGuarded(inner), nil
}

func newShootVerifier(logger *slog.Logger, cfg *ShootVerifierConfig) (shootverify.Verifier, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	if cfg.Mode == "mock" {
		logger.Warn("shoot token verifier in mock mode: accepting HMAC mock tokens for the seeded cluster",
			"cluster_id", cfg.LocalClusterID)
		return shootverify.NewMockVerifier([]byte(cfg.MockSecret), cfg.LocalClusterID, cfg.LocalOrganizationID), nil
	}

	if cfg.UsesGardener() {
		if cfg.GardenerKubeconfig == "" {
			return nil, fmt.Errorf("GARDENER_KUBECONFIG required for real mode")
		}
		client, err := gardener.New(cfg.GardenerKubeconfig, logger)
		if err != nil {
			return nil, fmt.Errorf("create gardener client: %w", err)
		}
		logger.Info("shoot token verifier: TokenReview via Gardener admin kubeconfigs")
		return shootverify.NewGardenerVerifier(gardener.NewAdminKubeconfigCache(client, logger)), nil
	}

	// The sandbox kubeconfig is mounted from an optional Secret created out of
	// band; a missing file means "not this mode", not a boot failure.
	if cfg.PluginSandboxKubeconfig != "" {
		if _, err := os.Stat(cfg.PluginSandboxKubeconfig); err == nil {
			v, err := shootverify.NewKubeconfigVerifier(cfg.PluginSandboxKubeconfig, cfg.LocalClusterID, cfg.LocalOrganizationID)
			if err != nil {
				return nil, fmt.Errorf("sandbox verifier: %w", err)
			}
			logger.Info("shoot token verifier: TokenReview on the plugin sandbox cluster",
				"kubeconfig", cfg.PluginSandboxKubeconfig, "cluster_id", cfg.LocalClusterID)
			return v, nil
		}
		logger.Warn("plugin sandbox kubeconfig not found",
			"kubeconfig", cfg.PluginSandboxKubeconfig)
	}

	logger.Warn("shoot token verifier unavailable: no Gardener, sandbox or mock verifier configured; every exchange will be denied")
	return denyAll{}, nil
}

// denyAll is the verifier of last resort when authn-api has no cluster to
// ask: local dev without the plugin sandbox, or `go run` outside Kubernetes.
type denyAll struct{}

func (denyAll) Verify(_ context.Context, _, _, _ string) (*shootverify.Subject, error) {
	return nil, fmt.Errorf("%w: no shoot token verifier configured", shootverify.ErrUnavailable)
}
