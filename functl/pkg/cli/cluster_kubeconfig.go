package cli

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ClusterKubeconfigCmd handles the cluster kubeconfig command.
type ClusterKubeconfigCmd struct {
	OrgID     string `help:"Organization ID." required:"" name:"org"`
	ClusterID string `arg:"" help:"Cluster ID to generate kubeconfig for."`
}

// Run executes the cluster kubeconfig command.
func (c *ClusterKubeconfigCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig(WithOrg(c.OrgID))
	if err != nil {
		return err
	}

	resp, err := apiClient.Clusters().GetKubeconfig(context.Background(), connect.NewRequest(organizationv1.GetKubeconfigRequest_builder{
		ClusterId: c.ClusterID,
	}.Build()))
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	fmt.Print(resp.Msg.GetKubeconfigContent())
	return nil
}
