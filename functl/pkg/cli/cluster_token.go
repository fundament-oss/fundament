package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// ClusterTokenCmd handles the cluster token command.
// It outputs an ExecCredential JSON for use as a kubectl credential plugin.
// The token is a Fundament platform JWT (cluster-agnostic); the proxy handles
// per-cluster SA token injection.
type ClusterTokenCmd struct {
	ClusterID string `arg:"" help:"Cluster ID (accepted for compatibility, not used for token exchange)."`
}

// Run executes the cluster token command.
func (c *ClusterTokenCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	token, expiry, err := apiClient.ExchangeToken(context.Background())
	if err != nil {
		return fmt.Errorf("failed to exchange API key for token: %w", err)
	}

	execCredential := map[string]any{
		"apiVersion": "client.authentication.k8s.io/v1",
		"kind":       "ExecCredential",
		"status": map[string]any{
			"token":               token,
			"expirationTimestamp": expiry.UTC().Format("2006-01-02T15:04:05Z"),
		},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(execCredential); err != nil {
		return fmt.Errorf("failed to encode exec credential: %w", err)
	}

	return nil
}
