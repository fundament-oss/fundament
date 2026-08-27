package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
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
	statusURL  string
	httpClient *http.Client

	mu     sync.RWMutex
	cached string
	store  string
}

// NewModelPin returns a pin reading the provisioner's status document. A pin with
// no status URL resolves to the empty id, which evaluates against the latest model.
func NewModelPin(statusURL string) *ModelPin {
	return &ModelPin{
		statusURL:  statusURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// ID returns the model id to evaluate against for the given store, fetching it
// once and reusing it until the store changes.
func (p *ModelPin) ID(ctx context.Context, storeID string) (string, error) {
	if p == nil || p.statusURL == "" {
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

// provisionStatus is the provisioner's status document. Only the fields consumers
// act on are decoded.
type provisionStatus struct {
	StoreID string `json:"id"`
	ModelID string `json:"model_id"`
}

func (p *ModelPin) fetch(ctx context.Context) (provisionStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.statusURL, http.NoBody)
	if err != nil {
		return provisionStatus{}, fmt.Errorf("build status request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return provisionStatus{}, fmt.Errorf("fetch provisioning status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return provisionStatus{}, fmt.Errorf("%w: status endpoint returned %d", ErrModelUnknown, resp.StatusCode)
	}

	var status provisionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return provisionStatus{}, fmt.Errorf("decode provisioning status: %w", err)
	}

	return status, nil
}
