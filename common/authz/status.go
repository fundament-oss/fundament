package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ProvisionStatus is what the OpenFGA provisioner publishes once it has the
// datastore in the shape this release expects.
type ProvisionStatus struct {
	// Generation names the release the datastore belongs to.
	Generation string `json:"generation"`
	StoreID    string `json:"id"`
}

// StatusClient reads that document.
//
// It is the only thing that can tell one release's store from another's: a store
// exists either way, and OpenFGA's Write does not check that a store survives, so
// existence alone lets a writer succeed against a store about to be replaced.
type StatusClient struct {
	url        string
	httpClient *http.Client
}

// NewStatusClient returns a client for the provisioner at url. A client with no
// url reports an empty generation, which leaves callers ungated.
func NewStatusClient(url string) *StatusClient {
	return &StatusClient{url: url, httpClient: &http.Client{Timeout: 5 * time.Second}}
}

// Generation reports which release the provisioner says the datastore belongs to.
// It is read fresh every time: a caller asking this is deciding whether the store
// in front of it is its own, and a cached answer would defeat that.
func (c *StatusClient) Generation(ctx context.Context) (string, error) {
	if c == nil || c.url == "" {
		return "", nil
	}

	status, err := c.Status(ctx)
	if err != nil {
		return "", err
	}

	return status.Generation, nil
}

// Status fetches the whole document.
func (c *StatusClient) Status(ctx context.Context) (ProvisionStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, http.NoBody)
	if err != nil {
		return ProvisionStatus{}, fmt.Errorf("build status request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ProvisionStatus{}, fmt.Errorf("fetch provisioning status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ProvisionStatus{}, fmt.Errorf("status endpoint returned %d", resp.StatusCode)
	}

	var status ProvisionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return ProvisionStatus{}, fmt.Errorf("decode provisioning status: %w", err)
	}

	return status, nil
}
