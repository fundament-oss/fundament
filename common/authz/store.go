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

// ErrNoStore reports that no store with the configured name exists.
var ErrNoStore = errors.New("openfga store not provisioned")

// Config holds configuration for the OpenFGA client.
type Config struct {
	APIURL string `env:"OPENFGA_API_URL,required,notEmpty"`
	// The store id is generated at creation, so services resolve it from this name.
	StoreName string `env:"OPENFGA_STORE_NAME,notEmpty" envDefault:"fundament"`
}

// Store caches the id of the store named in the config. The id is generated at
// creation, so it is looked up from the name.
type Store struct {
	fga  *client.OpenFgaClient
	name string

	mu sync.RWMutex
	id string
}

// NewStore references the store named in the config. The id is resolved on
// first use, so a service can start before OpenFGA is provisioned and report
// itself unready until it is.
func NewStore(cfg Config) (*Store, error) {
	fga, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: cfg.APIURL})
	if err != nil {
		return nil, fmt.Errorf("create OpenFGA client: %w", err)
	}

	return &Store{fga: fga, name: cfg.StoreName}, nil
}

// ID returns the cached id, resolving it the first time.
func (s *Store) ID(ctx context.Context) (string, error) {
	s.mu.RLock()
	id := s.id
	s.mu.RUnlock()

	if id != "" {
		return id, nil
	}

	return s.refresh(ctx)
}

// Healthy checks that the referenced store still exists, resolving it again by
// name when it does not.
//
// GetStore is the probe: Check answers from an in-process typesystem cache and
// cannot observe a replaced datastore.
func (s *Store) Healthy(ctx context.Context) error {
	id, err := s.ID(ctx)
	if err != nil {
		return fmt.Errorf("openfga: %w", err)
	}

	if err := s.exists(ctx, id); err == nil {
		return nil
	}

	fresh, err := s.refresh(ctx)
	if err != nil {
		return fmt.Errorf("openfga: %w", err)
	}

	if err := s.exists(ctx, fresh); err != nil {
		return fmt.Errorf("openfga: %w", err)
	}

	return nil
}

// refresh resolves the id again and replaces the cache, for when the cached
// store no longer exists.
func (s *Store) refresh(ctx context.Context) (string, error) {
	id, err := ResolveStoreID(ctx, s.fga, s.name)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.id = id
	s.mu.Unlock()

	return id, nil
}

func (s *Store) exists(ctx context.Context, id string) error {
	if _, err := s.fga.GetStore(ctx).Options(client.ClientGetStoreOptions{StoreId: &id}).Execute(); err != nil {
		return fmt.Errorf("get store %s: %w", id, err)
	}

	return nil
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
