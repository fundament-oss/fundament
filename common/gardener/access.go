package gardener

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/sync/singleflight"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// accessRefreshRatio is the fraction of the admin kubeconfig's TTL after which
// a cached ShootAccess is proactively refreshed in the background — the cached
// entry keeps being served (it is still valid) until it is refreshed or hard
// expires. 70% leaves generous headroom to retry a failed refresh several
// times before the credential expires.
const accessRefreshRatio = 0.7

// defaultFetchTimeout bounds a single admin-kubeconfig fetch from Gardener so
// a hung call cannot pin the singleflight (and every caller waiting on it)
// indefinitely.
const defaultFetchTimeout = 30 * time.Second

// ShootAccess is everything derived from one admin kubeconfig that callers
// need to talk to a shoot's API server.
type ShootAccess struct {
	// Host is the API server base URL (no path/query).
	Host *url.URL
	// Transport authenticates requests to the API server as the Gardener
	// admin (the admin kubeconfig's client certificate). Do not use it where
	// a per-request bearer token is meant to be the identity: a client cert
	// on the transport wins the apiserver auth chain and the injected token
	// is ignored. Consumers that inject SA tokens must build an anonymous
	// transport from RESTConfig instead (see kube-api-proxy's use of
	// rest.AnonymousClientConfig).
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
	client shootAccessSource
	logger *slog.Logger

	entries      *ttlcache.Cache[string, *ShootAccess]
	group        singleflight.Group // deduplicates concurrent fetches per cluster
	refreshing   sync.Map           // clusterID -> struct{}; at most one background refresh per cluster
	fetchTimeout time.Duration
}

// shootAccessSource is the subset of *Client that AdminKubeconfigCache uses,
// as an unexported seam so tests can substitute a fake.
type shootAccessSource interface {
	FindShoot(ctx context.Context, clusterID string) (*gardencorev1beta1.Shoot, error)
	AdminKubeconfigForShoot(ctx context.Context, shoot *gardencorev1beta1.Shoot, expirationSeconds int64) (*AdminKubeconfig, error)
}

// NewAdminKubeconfigCache returns a cache backed by the given Gardener client.
func NewAdminKubeconfigCache(c *Client, logger *slog.Logger) *AdminKubeconfigCache {
	return newAdminKubeconfigCache(c, logger)
}

func newAdminKubeconfigCache(src shootAccessSource, logger *slog.Logger) *AdminKubeconfigCache {
	entries := ttlcache.New[string, *ShootAccess](
		// Each entry gets its own TTL (the kubeconfig's remaining lifetime).
		ttlcache.WithTTL[string, *ShootAccess](ttlcache.NoTTL),
		// Reads must not extend an entry's lifetime: the TTL is the
		// kubeconfig's hard expiry, not a sliding window.
		ttlcache.WithDisableTouchOnHit[string, *ShootAccess](),
	)
	go entries.Start() // background eviction of hard-expired entries
	return &AdminKubeconfigCache{
		client:       src,
		logger:       logger,
		entries:      entries,
		fetchTimeout: defaultFetchTimeout,
	}
}

// AccessFor returns a valid ShootAccess for the cluster. A cached entry is
// served until its kubeconfig hard-expires; once past accessRefreshRatio of
// its lifetime it is refreshed in the background so requests never block on
// (or fail from) a transient Gardener error while a usable credential is in
// hand. A cache miss (or hard-expired entry) fetches synchronously.
func (a *AdminKubeconfigCache) AccessFor(ctx context.Context, clusterID string) (*ShootAccess, error) {
	item := a.entries.Get(clusterID)
	if item != nil {
		access := item.Value()
		if time.Now().After(access.refreshAt) {
			// At most one refresh goroutine per cluster: while Gardener is
			// unavailable refreshAt stays in the past, and without the guard
			// every request would park a new goroutine on the singleflight.
			if _, busy := a.refreshing.LoadOrStore(clusterID, struct{}{}); !busy {
				go func() {
					defer a.refreshing.Delete(clusterID)
					a.refresh(context.WithoutCancel(ctx), clusterID)
				}()
			}
		}
		return access, nil
	}
	return a.fetchAndCache(ctx, clusterID)
}

// fetchAndCache runs the singleflighted fetch. The fetch itself is detached
// from ctx (an abandoned request must not cancel the shared fetch for
// everyone else) and bounded by fetchTimeout, but the wait is not: when ctx
// is done the caller returns immediately while the shared fetch keeps
// running and populates the cache for the next request.
func (a *AdminKubeconfigCache) fetchAndCache(ctx context.Context, clusterID string) (*ShootAccess, error) {
	ch := a.group.DoChan(clusterID, func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.fetchTimeout)
		defer cancel()
		return a.fetch(fetchCtx, clusterID)
	})
	select {
	case res := <-ch:
		if res.Err != nil {
			return nil, fmt.Errorf("shoot access for cluster %s: %w", clusterID, res.Err)
		}
		return res.Val.(*ShootAccess), nil
	case <-ctx.Done():
		return nil, fmt.Errorf("shoot access for cluster %s: %w", clusterID, ctx.Err())
	}
}

// refresh re-fetches in the background; on failure it logs and leaves the
// still-valid cached entry in place (served until hard expiry). The caller
// passes a context already detached from the request (it must outlive the
// request that triggered it); fetchAndCache bounds the Gardener call with
// fetchTimeout.
func (a *AdminKubeconfigCache) refresh(ctx context.Context, clusterID string) {
	_, err := a.fetchAndCache(ctx, clusterID)
	if err != nil {
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
		return nil, fmt.Errorf("find shoot: %w", err)
	}

	adminKC, err := a.client.AdminKubeconfigForShoot(ctx, shoot, 0)
	if err != nil {
		return nil, fmt.Errorf("admin kubeconfig for shoot %s: %w", shoot.Name, err)
	}

	access, err := accessFromKubeconfig(adminKC.Kubeconfig)
	if err != nil {
		return nil, err
	}
	access.OrganizationID = shoot.Labels[LabelOrganizationID]

	ttl := time.Until(adminKC.ExpiresAt)
	if ttl <= 0 {
		// ttlcache treats ttl <= 0 as never-expires; never cache a dead
		// credential.
		return nil, fmt.Errorf("admin kubeconfig for cluster %s already expired at %s", clusterID, adminKC.ExpiresAt)
	}
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
