package authz

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

// storeRef caches the id of the store this client evaluates against. The id is
// generated at creation, so it is looked up from the name.
type storeRef struct {
	name string

	mu sync.RWMutex
	id string
}

func (s *storeRef) cached() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.id
}

func (s *storeRef) set(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.id = id
}

// resolve looks the store up and caches it.
func (s *storeRef) resolve(ctx context.Context, fga *client.OpenFgaClient) (string, error) {
	id, err := ResolveStoreID(ctx, fga, s.name)
	if err != nil {
		return "", err
	}

	s.set(id)

	return id, nil
}

// get returns the cached id, looking it up the first time.
func (s *storeRef) get(ctx context.Context, fga *client.OpenFgaClient) (string, error) {
	if id := s.cached(); id != "" {
		return id, nil
	}

	return s.resolve(ctx, fga)
}

// ResolveStoreID returns the id of the store called name, or ErrNoStore.
//
// OpenFGA does not make store names unique, so several can carry one name. The
// oldest wins because that ordering is total and does not shift: every service
// reaches the same store without coordinating. Soft-deleted stores are skipped —
// one still answers Check with allowed:true.
func ResolveStoreID(ctx context.Context, fga *client.OpenFgaClient, name string) (string, error) {
	var (
		matches []openfga.Store
		token   string
	)

	for {
		opts := client.ClientListStoresOptions{Name: &name}
		if token != "" {
			opts.ContinuationToken = &token
		}

		resp, err := fga.ListStores(ctx).Options(opts).Execute()
		if err != nil {
			return "", fmt.Errorf("list stores: %w", err)
		}

		for _, store := range resp.GetStores() {
			if store.Name == name && store.DeletedAt == nil {
				matches = append(matches, store)
			}
		}

		if token = resp.GetContinuationToken(); token == "" {
			break
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("%w: no store named %q", ErrNoStore, name)
	}

	oldest := slices.MinFunc(matches, func(a, b openfga.Store) int {
		if byAge := a.CreatedAt.Compare(b.CreatedAt); byAge != 0 {
			return byAge
		}

		// Created in the same instant: the id keeps the ordering total.
		return strings.Compare(a.Id, b.Id)
	})

	if len(matches) > 1 {
		ignored := make([]string, 0, len(matches)-1)

		for _, store := range matches {
			if store.Id != oldest.Id {
				ignored = append(ignored, store.Id)
			}
		}

		slog.WarnContext(ctx,
			"several OpenFGA stores share one name; using the oldest and ignoring the rest. "+
				"Only one is provisioned per environment, so the others were created by something else. "+
				"Tuples written against an ignored store are invisible to every service, "+
				"and permission checks against it answer false with nothing reporting an error.",
			"store_name", name,
			"using", oldest.Id,
			"ignoring", strings.Join(ignored, ","),
			"found", len(matches),
		)
	}

	return oldest.Id, nil
}
