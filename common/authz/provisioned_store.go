package authz

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// ErrNotProvisioned reports that the provisioner has not published a usable
// datastore yet, so there is nothing to evaluate against.
var ErrNotProvisioned = errors.New("openfga datastore not provisioned")

// ProvisionedStore is the OpenFGA store the provisioner put in force, read from
// what it published rather than worked out again.
//
// Deriving it a second time is what this replaces: the provisioner already
// resolved which store is current, and a consumer repeating that from the OpenFGA
// API has to reach the same answer by the same rules, or checks pass in one
// service and fail in another against different tuples.
//
// Replicas keep their own copies and never coordinate: after a reset each follows
// the new store on its next failed call, and one that has not caught up fails
// closed rather than answering from a store that is gone.
//
// This assumes one provisioner. Two on an empty datastore both create a store,
// publish different ids, and split consumers between them with every pod healthy.
// The chart pins openfga to a single replica for that reason.
type ProvisionedStore struct {
	status *StatusClient

	mu     sync.RWMutex
	cached ProvisionStatus
}

// NewProvisionedStore reads from the provisioner at statusURL.
func NewProvisionedStore(statusURL string) *ProvisionedStore {
	return &ProvisionedStore{status: NewStatusClient(statusURL)}
}

// Do runs op against the provisioned store.
//
// If op fails it re-reads the status once and retries only when the datastore has
// actually moved, which is what lets a consumer follow a reset without a restart.
// When nothing moved the original error stands rather than being retried for no
// reason.
func (p *ProvisionedStore) Do(ctx context.Context, op func(storeID string) error) error {
	current, err := p.get(ctx)
	if err != nil {
		return err
	}

	err = op(current.StoreID)
	if err == nil {
		return nil
	}

	fresh, refreshErr := p.refresh(ctx)
	if refreshErr != nil || fresh == current {
		return err
	}

	slog.Default().Info("openfga datastore moved, following it",
		"was", current.StoreID, "now", fresh.StoreID, "generation", fresh.Generation)

	return op(fresh.StoreID)
}

// Generation reports which release the datastore belongs to, read fresh: a caller
// asking this is deciding whether the datastore in front of it is its own, and a
// cached answer would defeat that.
func (p *ProvisionedStore) Generation(ctx context.Context) (string, error) {
	status, err := p.refresh(ctx)
	if err != nil {
		return "", err
	}

	return status.Generation, nil
}

func (p *ProvisionedStore) get(ctx context.Context) (ProvisionStatus, error) {
	p.mu.RLock()
	cached := p.cached
	p.mu.RUnlock()

	if cached.StoreID != "" {
		return cached, nil
	}

	return p.refresh(ctx)
}

func (p *ProvisionedStore) refresh(ctx context.Context) (ProvisionStatus, error) {
	status, err := p.status.Status(ctx)
	if err != nil {
		return ProvisionStatus{}, fmt.Errorf("%w: %w", ErrNotProvisioned, err)
	}

	if status.StoreID == "" {
		return ProvisionStatus{}, fmt.Errorf("%w: no store published", ErrNotProvisioned)
	}

	p.mu.Lock()
	p.cached = status
	p.mu.Unlock()

	return status, nil
}
