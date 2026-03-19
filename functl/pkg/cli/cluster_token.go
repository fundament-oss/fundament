package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// ClusterTokenCmd handles the cluster token command.
// It outputs an ExecCredential JSON for use as a kubectl credential plugin.
type ClusterTokenCmd struct {
	ClusterID string `arg:"" help:"Cluster ID to get a token for."`
}

// Run executes the cluster token command.
func (c *ClusterTokenCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	token, expiresAt, err := apiClient.ClusterToken(context.Background(), c.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster token: %w", err)
	}

	execCredential := map[string]any{
		"apiVersion": "client.authentication.k8s.io/v1beta1",
		"kind":       "ExecCredential",
		"status": map[string]any{
			"token":               token,
			"expirationTimestamp": expiresAt,
		},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(execCredential); err != nil {
		return fmt.Errorf("failed to encode exec credential: %w", err)
	}

	return nil
}
