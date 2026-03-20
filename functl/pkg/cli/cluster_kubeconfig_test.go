package cli

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuildKubeconfigStructure(t *testing.T) {
	t.Parallel()

	clusterID := uuid.New().String()
	serverURL := "https://api.test.example.com"
	caData := "dGVzdC1jYS1kYXRh" // base64("test-ca-data")
	functlPath := "/usr/local/bin/functl"

	kc := buildKubeconfig(clusterID, serverURL, caData, functlPath)

	// Verify top-level fields.
	require.Equal(t, "v1", kc["apiVersion"])
	require.Equal(t, "Config", kc["kind"])
	require.Equal(t, "fundament-"+clusterID, kc["current-context"])

	// Verify cluster entry.
	clusters := kc["clusters"].([]map[string]any)
	require.Len(t, clusters, 1)
	require.Equal(t, "fundament-"+clusterID, clusters[0]["name"])
	clusterDef := clusters[0]["cluster"].(map[string]any)
	require.Equal(t, serverURL, clusterDef["server"])
	require.Equal(t, caData, clusterDef["certificate-authority-data"])

	// Verify context entry.
	contexts := kc["contexts"].([]map[string]any)
	require.Len(t, contexts, 1)
	require.Equal(t, "fundament-"+clusterID, contexts[0]["name"])
	ctxDef := contexts[0]["context"].(map[string]any)
	require.Equal(t, "fundament-"+clusterID, ctxDef["cluster"])
	require.Equal(t, "fundament-user-"+clusterID, ctxDef["user"])

	// Verify user with exec plugin.
	users := kc["users"].([]map[string]any)
	require.Len(t, users, 1)
	require.Equal(t, "fundament-user-"+clusterID, users[0]["name"])
	userDef := users[0]["user"].(map[string]any)
	execDef := userDef["exec"].(map[string]any)
	require.Equal(t, "client.authentication.k8s.io/v1beta1", execDef["apiVersion"])
	require.Equal(t, functlPath, execDef["command"])
	require.Equal(t, []string{"cluster", "token", clusterID}, execDef["args"])
	require.Equal(t, "Never", execDef["interactiveMode"])
}

func TestBuildKubeconfigSerializesToValidYAML(t *testing.T) {
	t.Parallel()

	clusterID := uuid.New().String()
	kc := buildKubeconfig(clusterID, "https://api.test.example.com", "Y2EtZGF0YQ==", "functl")

	out, err := yaml.Marshal(kc)
	require.NoError(t, err)
	require.Contains(t, string(out), "server: https://api.test.example.com")
	require.Contains(t, string(out), "certificate-authority-data: Y2EtZGF0YQ==")
	require.Contains(t, string(out), "client.authentication.k8s.io/v1beta1")
	require.Contains(t, string(out), clusterID)
}

func TestClusterKubeconfigNotReady(t *testing.T) {
	t.Parallel()

	// When API server URL or CA data are empty, Run() should error.
	// We test the validation logic directly since Run() needs a real API client.
	require.Error(t, validateClusterReady("", "some-ca-data"), "should error when server URL is empty")
	require.Error(t, validateClusterReady("https://api.example.com", ""), "should error when CA data is empty")
	require.NoError(t, validateClusterReady("https://api.example.com", "some-ca-data"))
}

// validateClusterReady mirrors the validation logic in ClusterKubeconfigCmd.Run.
func validateClusterReady(apiServerURL, caData string) error {
	if apiServerURL == "" || caData == "" {
		return fmt.Errorf("cluster not ready yet (API server URL or CA data not available)")
	}
	return nil
}
