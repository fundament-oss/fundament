package authz

import (
	"context"
	"fmt"
	"sync"
)

// ModelPin resolves the authorization model to evaluate against, as reported by
// the provisioner that put it in force.
//
// OpenFGA evaluates the latest model when a request pins none, so any successful
// WriteAuthorizationModel silently becomes the rules every service enforces. The
// provisioner ships the model inside its own image and publishes the id it wrote,
// so pinning that id means a model written by anything else does not take effect
// merely by being newer.
type ModelPin struct {
	status *StatusClient

	mu     sync.RWMutex
	cached string
	store  string
}

// NewModelPin returns a pin reading the provisioner's status document. A pin with
// no status URL resolves to the empty id, which evaluates against the latest model.
func NewModelPin(statusURL string) *ModelPin {
	return &ModelPin{status: NewStatusClient(statusURL)}
}

// ID returns the model id to evaluate against for the given store, fetching it
// once and reusing it until the store changes.
func (p *ModelPin) ID(ctx context.Context, storeID string) (string, error) {
	if p == nil || p.status == nil || p.status.url == "" {
		return "", nil
	}

	p.mu.RLock()
	cached, forStore := p.cached, p.store
	p.mu.RUnlock()

	if cached != "" && forStore == storeID {
		return cached, nil
	}

	status, err := p.fetch(ctx)
	if err != nil {
		return "", err
	}

	// The published model belongs to the published store. Pinning it against a
	// different store would name a model that store never had, so a consumer that
	// has already moved on waits for the provisioner to catch up instead.
	if status.StoreID != storeID {
		return "", fmt.Errorf("%w: provisioner reports store %q, resolved %q",
			ErrModelUnknown, status.StoreID, storeID)
	}

	if status.ModelID == "" {
		return "", fmt.Errorf("%w: provisioner published no model id", ErrModelUnknown)
	}

	p.mu.Lock()
	p.cached, p.store = status.ModelID, status.StoreID
	p.mu.Unlock()

	return status.ModelID, nil
}

// fetch reads the status document, reporting any failure as ErrModelUnknown:
// to a caller deciding which model to evaluate against, an unreachable or
// unreadable provisioner and one publishing nothing are the same answer.
func (p *ModelPin) fetch(ctx context.Context) (ProvisionStatus, error) {
	status, err := p.status.Status(ctx)
	if err != nil {
		return ProvisionStatus{}, fmt.Errorf("%w: %w", ErrModelUnknown, err)
	}

	return status, nil
}
