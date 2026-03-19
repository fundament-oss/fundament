package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"connectrpc.com/connect"
	"gopkg.in/yaml.v3"

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

	resp, err := apiClient.Clusters().GetCluster(context.Background(), connect.NewRequest(organizationv1.GetClusterRequest_builder{
		ClusterId: c.ClusterID,
	}.Build()))
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	cluster := resp.Msg.GetCluster()

	apiServerURL := cluster.GetShootApiServerUrl()
	caData := cluster.GetShootCaData()

	if apiServerURL == "" || caData == "" {
		return fmt.Errorf("cluster not ready yet (API server URL or CA data not available)")
	}

	functlPath, err := os.Executable()
	if err != nil {
		functlPath = "functl"
	}

	kubeconfig := buildKubeconfig(c.ClusterID, apiServerURL, caData, functlPath)

	out, err := yaml.Marshal(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to marshal kubeconfig: %w", err)
	}

	fmt.Print(string(out))
	return nil
}

func buildKubeconfig(clusterID, apiServerURL, caData, functlPath string) map[string]any {
	clusterName := "fundament-" + clusterID
	contextName := clusterName
	userName := "fundament-user-" + clusterID

	// caData from the API is already base64-encoded
	caDataBytes, err := base64.StdEncoding.DecodeString(caData)
	if err != nil {
		// If it's not base64, use it as raw PEM and encode it
		caDataBytes = []byte(caData)
	}

	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Config",
		"clusters": []map[string]any{
			{
				"name": clusterName,
				"cluster": map[string]any{
					"server":                     apiServerURL,
					"certificate-authority-data": base64.StdEncoding.EncodeToString(caDataBytes),
				},
			},
		},
		"contexts": []map[string]any{
			{
				"name": contextName,
				"context": map[string]any{
					"cluster": clusterName,
					"user":    userName,
				},
			},
		},
		"current-context": contextName,
		"users": []map[string]any{
			{
				"name": userName,
				"user": map[string]any{
					"exec": map[string]any{
						"apiVersion":         "client.authentication.k8s.io/v1beta1",
						"command":            functlPath,
						"args":               []string{"cluster", "token", clusterID},
						"interactiveMode":    "Never",
						"provideClusterInfo": false,
					},
				},
			},
		},
	}
}
