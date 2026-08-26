package authz

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

// ErrStoreNotFound reports that no store with the configured name exists.
var ErrStoreNotFound = errors.New("openfga store not found")

// StoreResolver maps a store name to its id, caching the id and re-resolving
// when the store stops existing. Ids are server-generated and a reset replaces
// the store, so the name is the only stable handle.
type StoreResolver struct {
	fga  *client.OpenFgaClient
	name string

	mu     sync.RWMutex
	cached string
}

// NewStoreResolver returns a resolver for the store called name.
func NewStoreResolver(fga *client.OpenFgaClient, name string) *StoreResolver {
	return &StoreResolver{fga: fga, name: name}
}

// ID returns the store id, resolving it if nothing is cached yet.
func (r *StoreResolver) ID(ctx context.Context) (string, error) {
	r.mu.RLock()
	id := r.cached
	r.mu.RUnlock()

	if id != "" {
		return id, nil
	}

	return r.Resolve(ctx)
}

// Resolve looks the store up by name, bypassing the cache, so a store replaced
// by a reset is noticed.
func (r *StoreResolver) Resolve(ctx context.Context) (string, error) {
	// The lookup runs outside the lock: holding it across a round trip would
	// serialise every concurrent check behind one HTTP call whenever the cache is
	// empty. Concurrent resolves are harmless — they agree on the same store.
	var matches []openfga.Store

	// Follow the pages: the name filter is a server-side convenience, and a server
	// that ignores it puts the store on any page.
	for token := ""; ; {
		opts := client.ClientListStoresOptions{Name: &r.name}
		if token != "" {
			opts.ContinuationToken = &token
		}

		resp, err := r.fga.ListStores(ctx).Options(opts).Execute()
		if err != nil {
			return "", fmt.Errorf("list openfga stores: %w", err) // keep the cache: a transport failure says nothing about the store
		}

		for _, store := range resp.GetStores() {
			// Fail closed: never resolve a mismatched or soft-deleted store.
			if store.Name == r.name && store.DeletedAt == nil {
				matches = append(matches, store)
			}
		}

		if token = resp.GetContinuationToken(); token == "" {
			break
		}
	}

	if len(matches) == 0 {
		r.mu.Lock()
		r.cached = ""
		r.mu.Unlock()

		return "", fmt.Errorf("%w: %q", ErrStoreNotFound, r.name)
	}

	// Store names are not unique. Sort so every consumer independently picks the
	// same store; oldest wins — it is the one already holding tuples.
	slices.SortFunc(matches, func(a, b openfga.Store) int {
		if byAge := a.CreatedAt.Compare(b.CreatedAt); byAge != 0 {
			return byAge
		}

		return strings.Compare(a.Id, b.Id)
	})

	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.Id)
		}

		slog.Default().Warn("multiple openfga stores share one name, using the oldest",
			"name", r.name, "using", matches[0].Id, "found", strings.Join(ids, ","))
	}

	id := matches[0].Id

	r.mu.Lock()
	r.cached = id
	r.mu.Unlock()

	return id, nil
}

// Do runs op against the resolved store, re-resolving and retrying once if the
// store turned out to be gone.
func (r *StoreResolver) Do(ctx context.Context, op func(storeID string) error) error {
	id, err := r.ID(ctx)
	if err != nil {
		return err
	}

	if err = op(id); !storeGone(err) {
		return err
	}

	gone := id

	if id, err = r.Resolve(ctx); err != nil {
		slog.Default().Error("openfga store is gone and no replacement resolved",
			"name", r.name, "gone", gone, "error", err)

		return err
	}

	// The one visible sign that a reset happened: without it a store swap is
	// silent, and a consumer that failed to follow one looks identical to a
	// consumer that never had a store.
	slog.Default().Info("openfga store was replaced, following the new one",
		"name", r.name, "was", gone, "now", id)

	return op(id)
}

// storeGone reports whether err says the store id itself is unknown. OpenFGA
// returns 404 for a missing authorization model too, and re-resolving would not
// help there, so match on the response code rather than the status.
func storeGone(err error) bool {
	if err == nil {
		return false
	}

	var notFound openfga.FgaApiNotFoundError
	if !errors.As(err, &notFound) {
		return false
	}

	return notFound.ResponseCode() == openfga.NOTFOUNDERRORCODE_STORE_ID_NOT_FOUND
}
