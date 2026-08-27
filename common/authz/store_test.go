package authz

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Store ids must be well-formed ULIDs: the SDK rejects anything else before it
// reaches the wire.
const (
	storeOldest = "01JMZ0000000000000000000AA"
	storeNewer  = "01JMZ0000000000000000000BB"
	storeOther  = "01JMZ0000000000000000000CC"
)

func store(id, name string, created time.Time) openfga.Store {
	return openfga.Store{Id: id, Name: name, CreatedAt: created, UpdatedAt: created}
}

func deletedStore(id, name string, created time.Time) openfga.Store {
	s := store(id, name, created)
	at := created.Add(time.Minute)
	s.DeletedAt = &at

	return s
}

// fgaServer is a minimal stand-in for the store endpoints. The SDK retries a 404
// several times internally, so check responses key on the store id in the path.
type fgaServer struct {
	mu     sync.Mutex
	stores []openfga.Store
	status map[string]int
	// ignoreNameFilter models an older server that does not honour ?name=.
	ignoreNameFilter bool
	// pageSize splits the response across pages; 0 returns everything at once.
	pageSize int

	listCalls atomic.Int32
	// checkBody records the last check request, so tests can assert what went on
	// the wire rather than only what came back.
	checkBody atomic.Value
}

// lastCheckBody returns the body of the most recent check request.
func (s *fgaServer) lastCheckBody() string {
	body, _ := s.checkBody.Load().(string)

	return body
}

func (s *fgaServer) setStores(stores ...openfga.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stores = stores
}

func (s *fgaServer) markGone(ids ...string) {
	s.setStatus(http.StatusNotFound, ids...)
}

func (s *fgaServer) setStatus(code int, ids ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == nil {
		s.status = map[string]int{}
	}

	for _, id := range ids {
		s.status[id] = code
	}
}

func (s *fgaServer) start(t *testing.T) *client.OpenFgaClient {
	t.Helper()

	fga, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: s.url(t)})
	require.NoError(t, err)

	return fga
}

func (s *fgaServer) url(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if id, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/stores/"), "/check"); ok {
			if raw, err := io.ReadAll(r.Body); err == nil {
				s.checkBody.Store(string(raw))
			}

			s.mu.Lock()
			code := s.status[id]
			s.mu.Unlock()

			switch code {
			case 0, http.StatusOK:
				_, _ = w.Write([]byte(`{"allowed":true}`))
			case http.StatusNotFound:
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"code":"store_id_not_found","message":"store not found"}`))
			default:
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"code":"internal_error","message":"boom"}`))
			}

			return
		}

		s.listCalls.Add(1)
		_ = json.NewEncoder(w).Encode(s.list(r))
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

// list applies the name filter the way the real API does.
func (s *fgaServer) list(r *http.Request) openfga.ListStoresResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	matching := []openfga.Store{}

	for _, st := range s.stores {
		if name := r.URL.Query().Get("name"); s.ignoreNameFilter || name == "" || st.Name == name {
			matching = append(matching, st)
		}
	}

	if s.pageSize <= 0 {
		return openfga.ListStoresResponse{Stores: matching}
	}

	from, _ := strconv.Atoi(r.URL.Query().Get("continuation_token"))
	to := min(from+s.pageSize, len(matching))

	resp := openfga.ListStoresResponse{Stores: matching[from:to]}
	if to < len(matching) {
		resp.ContinuationToken = strconv.Itoa(to)
	}

	return resp
}

func newResolver(t *testing.T, srv *fgaServer) (*StoreResolver, *client.OpenFgaClient) {
	t.Helper()
	fga := srv.start(t)

	return NewStoreResolver(fga, "fundament"), fga
}

func TestResolveNoStoreWithThatName(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOther, "something-else", time.Now())}}
	r, _ := newResolver(t, srv)

	_, err := r.Resolve(t.Context())

	require.ErrorIs(t, err, ErrStoreNotFound)
	assert.Contains(t, err.Error(), `"fundament"`)
}

func TestResolveEmptyStoreList(t *testing.T) {
	r, _ := newResolver(t, &fgaServer{})

	_, err := r.Resolve(t.Context())

	require.ErrorIs(t, err, ErrStoreNotFound)
}

func TestResolveSkipsSoftDeletedStore(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{deletedStore(storeOldest, "fundament", time.Now())}}
	r, _ := newResolver(t, srv)

	_, err := r.Resolve(t.Context())

	require.ErrorIs(t, err, ErrStoreNotFound, "a store deleted by a reset must never resolve")
}

func TestResolveSkipsSoftDeletedStoreAndTakesTheLiveOne(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{stores: []openfga.Store{
		deletedStore(storeOldest, "fundament", now),
		store(storeNewer, "fundament", now.Add(time.Hour)),
	}}
	r, _ := newResolver(t, srv)

	got, err := r.Resolve(t.Context())

	require.NoError(t, err)
	assert.Equal(t, storeNewer, got, "age must not outrank being alive")
}

func TestResolveDuplicateNamesPicksOldestDeterministically(t *testing.T) {
	now := time.Now()
	// Newest first, so first-match-wins would pick the wrong one.
	srv := &fgaServer{stores: []openfga.Store{
		store(storeNewer, "fundament", now.Add(time.Hour)),
		store(storeOldest, "fundament", now),
	}}
	r, _ := newResolver(t, srv)

	first, err := r.Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, storeOldest, first)

	second, err := r.Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, first, second, "every consumer must independently pick the same store")
}

func TestResolveDuplicateNamesTieBreakOnID(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{stores: []openfga.Store{
		store(storeNewer, "fundament", now),
		store(storeOldest, "fundament", now),
	}}
	r, _ := newResolver(t, srv)

	got, err := r.Resolve(t.Context())

	require.NoError(t, err)
	assert.Equal(t, storeOldest, got)
}

func TestIDCachesAndResolveBypassesTheCache(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", time.Now())}}
	r, _ := newResolver(t, srv)

	first, err := r.ID(t.Context())
	require.NoError(t, err)
	second, err := r.ID(t.Context())
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, int32(1), srv.listCalls.Load(), "a cached id must not cost a request")

	_, err = r.Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(2), srv.listCalls.Load())
}

func TestStaleIDDoesNotSurviveTheStoreItNames(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", time.Now())}}
	r, _ := newResolver(t, srv)

	_, err := r.ID(t.Context())
	require.NoError(t, err)

	srv.setStores() // the reset deleted it

	_, err = r.Resolve(t.Context())
	require.ErrorIs(t, err, ErrStoreNotFound)

	_, err = r.ID(t.Context())
	require.ErrorIs(t, err, ErrStoreNotFound)
}

func TestUnreachableServerKeepsTheCachedID(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", time.Now())}}
	fga := srv.start(t)
	r := NewStoreResolver(fga, "fundament")

	_, err := r.ID(t.Context())
	require.NoError(t, err)

	// A cancelled context stands in for an unreachable server.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = r.Resolve(ctx)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrStoreNotFound, "a transport failure is not evidence the store is gone")

	got, err := r.ID(t.Context())
	require.NoError(t, err)
	assert.Equal(t, storeOldest, got, "the cache must survive a blip")
}

// check drives a real SDK call so the retry path sees a genuine SDK error.
func check(fga *client.OpenFgaClient, storeID string) error {
	_, err := fga.Check(context.Background()).
		Body(client.ClientCheckRequest{User: "user:a", Relation: "can_view", Object: "organization:b"}).
		Options(client.ClientCheckOptions{StoreId: &storeID}).
		Execute()

	return err //nolint:wrapcheck // the resolver must see the raw SDK error
}

func TestDoRetriesOnceAgainstTheReplacementStore(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", now)}}
	r, fga := newResolver(t, srv)

	_, err := r.ID(t.Context())
	require.NoError(t, err)

	// A reset: the old store is gone, a new one has taken the name.
	srv.setStores(store(storeNewer, "fundament", now.Add(time.Hour)))
	srv.markGone(storeOldest)

	var used []string

	err = r.Do(t.Context(), func(storeID string) error {
		used = append(used, storeID)

		return check(fga, storeID)
	})

	require.NoError(t, err)
	assert.Equal(t, []string{storeOldest, storeNewer}, used, "the retry must run against the new store")
}

func TestDoRetriesOnlyOnce(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", now)}}
	r, fga := newResolver(t, srv)

	_, err := r.ID(t.Context())
	require.NoError(t, err)

	srv.setStores(store(storeNewer, "fundament", now.Add(time.Hour)))
	srv.markGone(storeOldest, storeNewer)

	attempts := 0
	err = r.Do(t.Context(), func(storeID string) error {
		attempts++

		return check(fga, storeID)
	})

	require.Error(t, err)
	assert.Equal(t, 2, attempts)
}

func TestDoSurfacesNotFoundWhenTheStoreIsGoneForGood(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", time.Now())}}
	r, fga := newResolver(t, srv)

	_, err := r.ID(t.Context())
	require.NoError(t, err)

	srv.setStores()
	srv.markGone(storeOldest)

	err = r.Do(t.Context(), func(storeID string) error { return check(fga, storeID) })

	require.ErrorIs(t, err, ErrStoreNotFound)
}

func TestResolveMatchesByNameWhenServerIgnoresTheFilter(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{
		ignoreNameFilter: true,
		stores: []openfga.Store{
			store(storeOther, "something-else", now.Add(-time.Hour)),
			store(storeOldest, "fundament", now),
		},
	}
	r, _ := newResolver(t, srv)

	got, err := r.Resolve(t.Context())

	require.NoError(t, err)
	assert.Equal(t, storeOldest, got, "an unrelated store must never be picked up")
}

func TestConcurrentResolversAgreeOnOneStore(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{stores: []openfga.Store{
		store(storeNewer, "fundament", now.Add(time.Hour)),
		store(storeOldest, "fundament", now),
	}}
	r, _ := newResolver(t, srv)

	const goroutines = 32

	var (
		wg  sync.WaitGroup
		ids = make([]string, goroutines)
	)

	for i := range goroutines {
		wg.Add(1)

		go func() {
			defer wg.Done()

			id, err := r.ID(t.Context())
			assert.NoError(t, err)
			ids[i] = id
		}()
	}

	wg.Wait()

	for i, id := range ids {
		assert.Equal(t, storeOldest, id, "goroutine %d disagreed", i)
	}
}

func TestDoSucceedsWithoutRetrying(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", time.Now())}}
	r, fga := newResolver(t, srv)

	attempts := 0
	err := r.Do(t.Context(), func(storeID string) error {
		attempts++

		return check(fga, storeID)
	})

	require.NoError(t, err)
	assert.Equal(t, 1, attempts)
	assert.Equal(t, int32(1), srv.listCalls.Load(), "a healthy call must not re-resolve")
}

func TestDoDoesNotRetryOnErrorsThatAreNotAMissingStore(t *testing.T) {
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", time.Now())}}
	r, fga := newResolver(t, srv)
	srv.setStatus(http.StatusInternalServerError, storeOldest)

	attempts := 0
	err := r.Do(t.Context(), func(storeID string) error {
		attempts++

		return check(fga, storeID)
	})

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrStoreNotFound)
	assert.Equal(t, 1, attempts, "a failing store is not a replaced store")
}

func TestClientHealthyReportsAMissingStore(t *testing.T) {
	srv := &fgaServer{}
	c, err := New(Config{APIURL: srv.url(t), StoreName: "fundament"})
	require.NoError(t, err)

	require.ErrorIs(t, c.Healthy(t.Context()), ErrStoreNotFound)
}

func TestClientHealthyNoticesAReplacedStore(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", now)}}
	c, err := New(Config{APIURL: srv.url(t), StoreName: "fundament"})
	require.NoError(t, err)
	require.NoError(t, c.Healthy(t.Context()))

	srv.setStores()
	require.ErrorIs(t, c.Healthy(t.Context()), ErrStoreNotFound, "readiness is what notices the swap")

	srv.setStores(store(storeNewer, "fundament", now.Add(time.Hour)))
	require.NoError(t, c.Healthy(t.Context()))

	id, err := c.store.ID(t.Context())
	require.NoError(t, err)
	assert.Equal(t, storeNewer, id)
}

func TestClientEvaluateFollowsAReplacedStore(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{stores: []openfga.Store{store(storeOldest, "fundament", now)}}
	c, err := New(Config{APIURL: srv.url(t), StoreName: "fundament"})
	require.NoError(t, err)

	_, err = c.store.ID(t.Context())
	require.NoError(t, err)

	srv.setStores(store(storeNewer, "fundament", now.Add(time.Hour)))
	srv.markGone(storeOldest)

	dec, err := c.Evaluate(t.Context(), EvaluationRequest{
		Subject:  User(uuid.New()),
		Action:   CanView(),
		Resource: Cluster(uuid.New()),
	})

	require.NoError(t, err, "a check must survive a reset without a restart")
	assert.True(t, dec.Decision)
}

func TestClientEvaluateDeniesWhenNoStoreExists(t *testing.T) {
	c, err := New(Config{APIURL: (&fgaServer{}).url(t), StoreName: "fundament"})
	require.NoError(t, err)

	dec, err := c.Evaluate(t.Context(), EvaluationRequest{
		Subject:  User(uuid.New()),
		Action:   CanView(),
		Resource: Cluster(uuid.New()),
	})

	require.ErrorIs(t, err, ErrStoreNotFound)
	assert.False(t, dec.Decision, "a missing store must deny, never allow")
}

func TestResolveFollowsPagination(t *testing.T) {
	now := time.Now()
	// The match sits on the last page, and the pages arrive newest-first, so a
	// resolver reading only the first page would both miss it and, once it read
	// far enough, pick the wrong one.
	srv := &fgaServer{
		pageSize: 1,
		stores: []openfga.Store{
			store(storeOther, "something-else", now),
			store(storeNewer, "fundament", now),
			store(storeOldest, "fundament", now.Add(-time.Hour)),
		},
	}
	r, _ := newResolver(t, srv)

	id, err := r.Resolve(t.Context())

	require.NoError(t, err)
	assert.Equal(t, storeOldest, id)
	assert.Equal(t, int32(2), srv.listCalls.Load()) // two matches, one per page
}

func TestResolveFollowsPaginationWhenServerIgnoresNameFilter(t *testing.T) {
	now := time.Now()
	srv := &fgaServer{
		pageSize:         1,
		ignoreNameFilter: true,
		stores: []openfga.Store{
			store(storeOther, "something-else", now.Add(-2*time.Hour)),
			deletedStore(storeNewer, "fundament", now.Add(-3*time.Hour)),
			store(storeOldest, "fundament", now),
		},
	}
	r, _ := newResolver(t, srv)

	id, err := r.Resolve(t.Context())

	require.NoError(t, err)
	assert.Equal(t, storeOldest, id)
}
