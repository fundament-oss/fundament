package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/sync/singleflight"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/shootidentity"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/gardener"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kube"
	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/kubereq"
)

// Namespace listing for GUI tools (k9s, Lens, Headlamp): Kubernetes can't grant
// "list the namespaces I may see", so a project member's GET /api/v1/namespaces
// would be a 403. The proxy serves it as the user's own ServiceAccount plus a
// group that may read namespaces, restricted by a label selector to the
// namespaces of the projects the user can view. Org admins get no selector.
// The response is streamed untouched, so tables, protobuf and watch all work.

const (
	// visibilityTTL bounds how stale a user's visible project set may be.
	visibilityTTL = 30 * time.Second

	// maxFilteredWatchSeconds caps non-admin namespace watches, so a changed
	// visible set takes effect when the client reconnects.
	maxFilteredWatchSeconds = 300

	// noVisibleProjects is an Exists requirement on a label nothing carries: an
	// empty result set rather than a 403.
	noVisibleProjects = "fundament.io/no-visible-projects"
)

// visibility is what a user may see in a cluster's namespace listing.
type visibility struct {
	admin      bool     // every namespace
	projectIDs []string // otherwise: namespaces of these projects only
}

// visibilityAuthz is the slice of *authz.Client the resolver needs.
type visibilityAuthz interface {
	Evaluate(ctx context.Context, req authz.EvaluationRequest) (authz.Decision, error)
	ListObjects(ctx context.Context, subject authz.Object, action authz.Action, objectType authz.ObjectType) ([]string, error)
}

type visibilityKey struct {
	userID    uuid.UUID
	clusterID uuid.UUID
}

// visibilityResolver answers "what may this user see on this cluster" from
// OpenFGA, cached briefly per (user, cluster).
type visibilityResolver struct {
	authz visibilityAuthz
	cache *ttlcache.Cache[visibilityKey, visibility]
	group singleflight.Group
}

func newVisibilityResolver(az visibilityAuthz) *visibilityResolver {
	v := &visibilityResolver{
		authz: az,
		cache: ttlcache.New(ttlcache.WithTTL[visibilityKey, visibility](visibilityTTL), ttlcache.WithDisableTouchOnHit[visibilityKey, visibility]()),
	}
	go v.cache.Start() // evict expired entries
	return v
}

func (v *visibilityResolver) resolve(ctx context.Context, userID, clusterID uuid.UUID) (visibility, error) {
	key := visibilityKey{userID: userID, clusterID: clusterID}
	if item := v.cache.Get(key); item != nil {
		return item.Value(), nil
	}

	res, err, _ := v.group.Do(userID.String()+"/"+clusterID.String(), func() (any, error) {
		vis, err := v.lookup(ctx, userID, clusterID)
		if err != nil {
			return visibility{}, err
		}
		v.cache.Set(key, vis, ttlcache.DefaultTTL)
		return vis, nil
	})
	if err != nil {
		return visibility{}, fmt.Errorf("resolve namespace visibility: %w", err)
	}
	return res.(visibility), nil
}

func (v *visibilityResolver) lookup(ctx context.Context, userID, clusterID uuid.UUID) (visibility, error) {
	dec, err := v.authz.Evaluate(ctx, authz.EvaluationRequest{
		Subject:  authz.User(userID),
		Action:   authz.Admin(),
		Resource: authz.Cluster(clusterID),
	})
	if err != nil {
		return visibility{}, fmt.Errorf("check cluster admin: %w", err)
	}
	if dec.Decision {
		return visibility{admin: true}, nil
	}

	ids, err := v.authz.ListObjects(ctx, authz.User(userID), authz.CanView(), authz.ObjectTypeProject)
	if err != nil {
		return visibility{}, fmt.Errorf("list viewable projects: %w", err)
	}
	slices.Sort(ids)
	return visibility{projectIDs: ids}, nil
}

// isNamespaceCollectionGet reports whether r lists or watches namespaces.
// Single-namespace requests (/api/v1/namespaces/{name}) are not matched: the
// user's own RoleBindings authorize those.
func isNamespaceCollectionGet(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces"
}

// projectRequirement is the label requirement restricting a listing to vis;
// empty for admins.
func projectRequirement(vis visibility) string {
	switch {
	case vis.admin:
		return ""
	case len(vis.projectIDs) == 0:
		return noVisibleProjects
	default:
		return shootidentity.LabelProjectID + " in (" + strings.Join(vis.projectIDs, ",") + ")"
	}
}

// rewriteNamespaceQuery ANDs the project requirement into the client's label
// selector and caps non-admin watches.
func rewriteNamespaceQuery(r *http.Request, vis visibility) {
	q := r.URL.Query()
	if req := projectRequirement(vis); req != "" {
		if existing := q.Get("labelSelector"); existing != "" {
			req = existing + "," + req
		}
		q.Set("labelSelector", req)
	}
	if !vis.admin && isWatch(r) {
		n, err := strconv.Atoi(q.Get("timeoutSeconds"))
		if err != nil || n <= 0 || n > maxFilteredWatchSeconds {
			q.Set("timeoutSeconds", strconv.Itoa(maxFilteredWatchSeconds))
		}
	}
	r.URL.RawQuery = q.Encode()
}

// isWatch reuses the apiserver-compatible watch detection of kubereq.
func isWatch(r *http.Request) bool {
	attrs, err := kubereq.Parse(r)
	return err == nil && attrs.Verb == "watch"
}

// namespaceLister serves filtered namespace listings.
type namespaceLister struct {
	logger     *slog.Logger
	visibility *visibilityResolver
	proxyToken func(ctx context.Context, clusterID string) (string, error)
	upstream   http.Handler
}

func (n *namespaceLister) serve(w http.ResponseWriter, r *http.Request, userID, clusterID uuid.UUID) {
	ctx := r.Context()

	vis, err := n.visibility.resolve(ctx, userID, clusterID)
	if err != nil {
		n.logger.ErrorContext(ctx, "resolve namespace visibility", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	token, err := n.proxyToken(ctx, clusterID.String())
	if err != nil {
		if errors.Is(err, gardener.ErrSyncPending) {
			http.Error(w, "service account sync pending, try again shortly", http.StatusServiceUnavailable)
			return
		}
		n.logger.ErrorContext(ctx, "get kube-api-proxy SA token", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	r = r.Clone(ctx)
	r.URL = cloneURL(r.URL)
	rewriteNamespaceQuery(r, vis)

	n.logger.InfoContext(ctx, "serving namespace listing",
		"user_id", userID, "cluster_id", clusterID, "admin", vis.admin,
		"projects", len(vis.projectIDs), "query", r.URL.RawQuery)

	ctx = WithSAToken(ctx, token)
	ctx = kube.WithImpersonation(ctx, kube.Impersonation{
		User:   shootidentity.UserServiceAccountUsername(userID.String()),
		Groups: []string{shootidentity.NamespaceListerGroup},
	})
	ctx = context.WithValue(ctx, kube.ClusterIDContextKey{}, clusterID.String())
	n.upstream.ServeHTTP(w, r.WithContext(ctx))
}

func cloneURL(u *url.URL) *url.URL {
	c := *u
	return &c
}
