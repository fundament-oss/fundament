// Package pluginmachinery provisions the plugin substrate onto every ready
// shoot: the PluginInstallation CRD, and the plugin-controller with its
// ServiceAccount, ClusterRole and ClusterRoleBinding in fundament-system. It
// mirrors the registration shape of usersync/namespace-sync — subscribed to
// the cluster-ready outbox event, re-asserted by the periodic reconcile loop —
// so the machinery both appears as soon as a shoot is ready and heals if it
// is removed by hand.
//
// The console writes PluginInstallation CRs straight onto shoots via
// kube-api-proxy; without this handler those writes 404 because nothing else
// ever installs the CRD or the controller there (Helm cannot reach a shoot).
package pluginmachinery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/pluginmachinery/manifests"
)

// Config holds the environment-level inputs of the shoot-side
// plugin-controller. ControllerImage and CatalogAPIURL have no sane
// defaults (they are deployment-specific); when either is empty the handler
// no-ops, so environments without plugin support (mock Gardener, PR previews)
// keep working without extra configuration.
type Config struct {
	// ControllerImage is the plugin-controller image, resolvable from shoot nodes.
	ControllerImage string `env:"CONTROLLER_IMAGE"`
	// CatalogAPIURL is the externally routable marketplace-catalog-api base URL.
	CatalogAPIURL string `env:"MARKETPLACE_CATALOG_API_URL"`
	// AllowUnpinnedHash disables definition-hash verification on the shoot-side
	// controller. Local dev only; never enable for production shoots.
	AllowUnpinnedHash bool `env:"ALLOW_UNPINNED_HASH"`
	// LogLevel is the shoot-side controller's LOG_LEVEL (empty = its default).
	LogLevel string `env:"LOG_LEVEL"`
}

// Enabled reports whether the handler has the config it needs to provision.
func (c Config) Enabled() bool {
	return c.ControllerImage != "" && c.CatalogAPIURL != ""
}

// Handler provisions the plugin machinery onto ready shoots.
type Handler struct {
	queries *db.Queries
	shoot   shoot.ShootAccess
	cfg     Config
	logger  *slog.Logger

	logDisabledOnce sync.Once
}

// New constructs a pluginmachinery handler.
func New(pool *pgxpool.Pool, shootAccess shoot.ShootAccess, cfg Config, logger *slog.Logger) *Handler {
	return &Handler{
		queries: db.New(pool),
		shoot:   shootAccess,
		cfg:     cfg,
		logger:  logger.With("handler", "pluginmachinery"),
	}
}

// Sync handles the cluster-ready outbox event: the row id is the cluster id.
func (h *Handler) Sync(ctx context.Context, id uuid.UUID, sc handler.SyncContext) error {
	switch sc.EntityType {
	case handler.EntityCluster:
		return h.provision(ctx, id)
	default:
		return fmt.Errorf("unexpected entity type %s for pluginmachinery handler", sc.EntityType)
	}
}

// Reconcile re-asserts the machinery on every ready cluster, isolating
// per-cluster failures so one sick shoot does not block the rest.
func (h *Handler) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // graceful shutdown
	}
	if !h.cfg.Enabled() {
		h.logDisabled()
		return nil
	}

	clusterIDs, err := h.queries.ClusterListReady(ctx)
	if err != nil {
		return fmt.Errorf("list ready clusters: %w", err)
	}

	var errs []error
	for _, clusterID := range clusterIDs {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // graceful shutdown
		}
		if err := h.provision(ctx, clusterID); err != nil {
			// A cluster that stopped being ready between ClusterListReady and
			// the per-cluster re-read is a benign race, not a reconcile
			// failure: counting it as one feeds the reconcile worker's
			// consecutive-failure counter, which exits the process at three.
			// The cluster-ready event and the next tick re-run it.
			if precond, ok := errors.AsType[*handler.PreconditionError](err); ok {
				h.logger.Info("skipping cluster for plugin machinery",
					"cluster_id", clusterID,
					"reason", precond.Reason)
				continue
			}
			h.logger.Error("failed to provision plugin machinery",
				"cluster_id", clusterID,
				"error", err)
			errs = append(errs, err)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("plugin machinery reconcile: %w", err)
	}
	return nil
}

// provision converges one shoot to the desired plugin machinery state. Every
// step is idempotent, so re-runs (ready event + reconcile loop) are safe.
func (h *Handler) provision(ctx context.Context, clusterID uuid.UUID) error {
	if !h.cfg.Enabled() {
		h.logDisabled()
		return nil
	}

	row, err := h.queries.ClusterGetForPluginMachinery(ctx, db.ClusterGetForPluginMachineryParams{ID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("cluster not found or deleted, skipping", "cluster_id", clusterID)
			return nil
		}
		return fmt.Errorf("get cluster for plugin machinery: %w", err)
	}

	// Gate on shoot readiness — defer (don't fail) so a stray event does not
	// burn outbox retries; the cluster-ready fan-out and the reconcile loop
	// re-run this once the shoot is up. Mirrors the namespace handler.
	if !row.ShootStatus.Valid || row.ShootStatus.String != string(gardener.StatusReady) {
		return handler.NewPreconditionError("shoot not ready")
	}

	params := &manifests.DeploymentParams{
		Image:             h.cfg.ControllerImage,
		ClusterID:         clusterID.String(),
		OrganizationID:    row.OrganizationID.String(),
		CatalogAPIURL:     h.cfg.CatalogAPIURL,
		AllowUnpinnedHash: h.cfg.AllowUnpinnedHash,
		LogLevel:          h.cfg.LogLevel,
	}
	if err := params.Validate(); err != nil {
		return fmt.Errorf("deployment params: %w", err)
	}

	labels := manifests.Labels()

	// One set of shoot credentials for the whole batch: without this each verb
	// below mints its own admin kubeconfig, six per cluster per pass.
	ctx = shoot.WithClientCache(ctx)

	if err := h.shoot.EnsureCRD(ctx, clusterID, manifests.CRD); err != nil {
		return fmt.Errorf("ensure PluginInstallation CRD: %w", err)
	}
	if err := h.shoot.EnsureNamespace(ctx, clusterID, manifests.Namespace); err != nil {
		return fmt.Errorf("ensure namespace: %w", err)
	}
	if err := h.shoot.EnsureServiceAccount(ctx, clusterID, manifests.Namespace, manifests.DeploymentName, labels, nil); err != nil {
		return fmt.Errorf("ensure ServiceAccount: %w", err)
	}
	if err := h.shoot.EnsureClusterRole(ctx, clusterID, manifests.ClusterRoleName, manifests.ClusterRoleRules(), labels); err != nil {
		return fmt.Errorf("ensure ClusterRole: %w", err)
	}
	if err := h.shoot.EnsureClusterRoleBinding(ctx, clusterID, manifests.ClusterRoleName, manifests.ClusterRoleName, manifests.Namespace, manifests.DeploymentName, labels, nil); err != nil {
		return fmt.Errorf("ensure ClusterRoleBinding: %w", err)
	}
	if err := h.shoot.EnsureDeployment(ctx, clusterID, manifests.Deployment(params)); err != nil {
		return fmt.Errorf("ensure Deployment: %w", err)
	}

	h.logger.Info("provisioned plugin machinery", "cluster_id", clusterID)
	return nil
}

// logDisabled logs the disabled state once per process so a permanently
// unconfigured environment doesn't emit a line every reconcile tick.
func (h *Handler) logDisabled() {
	h.logDisabledOnce.Do(func() {
		h.logger.Info("plugin machinery provisioning disabled",
			"reason", "PLUGIN_CONTROLLER_IMAGE and/or PLUGIN_MARKETPLACE_CATALOG_API_URL not set")
	})
}
