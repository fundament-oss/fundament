package gardener

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/sync/singleflight"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// accessRefreshRatio is the fraction of the admin kubeconfig's TTL after which
// a cached ShootAccess is proactively refreshed in the background — the cached
// entry keeps being served (it is still valid) until it is refreshed or hard
// expires. Matches kube-api-proxy's token-cache convention.
const accessRefreshRatio = 0.7

// ShootAccess is everything derived from one admin kubeconfig that callers
// need to talk to a shoot's API server.
type ShootAccess struct {
	// Host is the API server base URL (no path/query).
	Host *url.URL
	// Transport authenticates requests to the API server.
	Transport http.RoundTripper
	// RESTConfig builds typed or controller-runtime clients for the shoot.
	RESTConfig *rest.Config
	// OrganizationID is the owning organization from the shoot's labels
	// (empty if the label is missing).
	OrganizationID string

	refreshAt time.Time
}

// AdminKubeconfigCache caches per-cluster ShootAccess derived from short-lived
// admin kubeconfigs. Entries are held in a TTL cache keyed to the kubeconfig's
// hard expiry (so stale entries for deleted/rotated clusters are evicted
// automatically), refreshed proactively in the background before expiry, and
// concurrent fetches for the same cluster are deduplicated via singleflight.
type AdminKubeconfigCache struct {
	client *Client
	logger *slog.Logger

	entries *ttlcache.Cache[string, *ShootAccess]
	group   singleflight.Group // deduplicates concurrent fetches per cluster
}

// NewAdminKubeconfigCache returns a cache backed by the given Gardener client.
func NewAdminKubeconfigCache(c *Client, logger *slog.Logger) *AdminKubeconfigCache {
	entries := ttlcache.New[string, *ShootAccess](
		// Each entry gets its own TTL (the kubeconfig's remaining lifetime).
		ttlcache.WithTTL[string, *ShootAccess](ttlcache.NoTTL),
	)
	go entries.Start() // background eviction of hard-expired entries
	return &AdminKubeconfigCache{client: c, logger: logger, entries: entries}
}

// AccessFor returns a valid ShootAccess for the cluster. A cached entry is
// served until its kubeconfig hard-expires; once past accessRefreshRatio of
// its lifetime it is refreshed in the background so requests never block on
// (or fail from) a transient Gardener error while a usable credential is in
// hand. A cache miss (or hard-expired entry) fetches synchronously.
func (a *AdminKubeconfigCache) AccessFor(ctx context.Context, clusterID string) (*ShootAccess, error) {
	if item := a.entries.Get(clusterID); item != nil {
		access := item.Value()
		if time.Now().After(access.refreshAt) {
			// Detach from the caller's context so an abandoned request cannot
			// cancel the shared refresh for everyone else.
			go a.refresh(context.WithoutCancel(ctx), clusterID)
		}
		return access, nil
	}
	return a.fetchAndCache(context.WithoutCancel(ctx), clusterID)
}

func (a *AdminKubeconfigCache) fetchAndCache(ctx context.Context, clusterID string) (*ShootAccess, error) {
	v, err, _ := a.group.Do(clusterID, func() (any, error) {
		return a.fetch(ctx, clusterID)
	})
	if err != nil {
		return nil, fmt.Errorf("shoot access for cluster %s: %w", clusterID, err)
	}
	return v.(*ShootAccess), nil
}

// refresh re-fetches in the background; on failure it logs and leaves the
// still-valid cached entry in place (served until hard expiry).
func (a *AdminKubeconfigCache) refresh(ctx context.Context, clusterID string) {
	if _, err := a.fetchAndCache(ctx, clusterID); err != nil {
		a.logger.WarnContext(ctx, "background shoot-access refresh failed; serving cached entry",
			"cluster_id", clusterID, "error", err)
	}
}

func (a *AdminKubeconfigCache) fetch(ctx context.Context, clusterID string) (*ShootAccess, error) {
	// Resolve the shoot once and reuse it for both the org-id label and the
	// adminkubeconfig subresource — a single List, and an atomic read (the org
	// id and the kubeconfig come from the same shoot object).
	shoot, err := a.client.FindShoot(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	adminKC, err := a.client.AdminKubeconfigForShoot(ctx, shoot, 0)
	if err != nil {
		return nil, err
	}

	access, err := accessFromKubeconfig(adminKC.Kubeconfig)
	if err != nil {
		return nil, err
	}
	access.OrganizationID = shoot.Labels[LabelOrganizationID]

	ttl := time.Until(adminKC.ExpiresAt)
	access.refreshAt = time.Now().Add(time.Duration(float64(ttl) * accessRefreshRatio))

	a.entries.Set(clusterID, access, ttl) // evicted at hard expiry
	a.logger.DebugContext(ctx, "shoot access cached",
		"cluster_id", clusterID, "refresh_at", access.refreshAt)
	return access, nil
}

// accessFromKubeconfig parses an admin kubeconfig into a ShootAccess.
// Auth is handled by the transport created via rest.TransportFor, which
// supports bearer tokens and client certificates from the kubeconfig.
func accessFromKubeconfig(kubeconfig []byte) (*ShootAccess, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build transport: %w", err)
	}

	host, err := url.Parse(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig host: %w", err)
	}
	host.Path = ""
	host.RawQuery = ""

	return &ShootAccess{
		Host:       host,
		Transport:  transport,
		RESTConfig: cfg,
	}, nil
}
