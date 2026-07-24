package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPluginInstallationResource_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1 is set")
	}

	if os.Getenv("FUNDAMENT_API_KEY") == "" {
		t.Fatal("FUNDAMENT_API_KEY must be set for acceptance tests")
	}

	endpoint := os.Getenv("FUNDAMENT_ENDPOINT")
	if endpoint == "" {
		t.Fatal("FUNDAMENT_ENDPOINT must be set for acceptance tests")
	}

	organizationID := os.Getenv("FUNDAMENT_ORGANIZATION_ID")
	if organizationID == "" {
		t.Fatal("FUNDAMENT_ORGANIZATION_ID must be set for acceptance tests")
	}

	// Skip (not Fatal) so CI without a kube proxy doesn't fail the test run.
	kubeProxyURL := os.Getenv("FUNDAMENT_KUBE_API_PROXY_URL")
	if kubeProxyURL == "" {
		t.Skip("FUNDAMENT_KUBE_API_PROXY_URL must be set for plugin installation acceptance tests")
	}

	suffix := acctest.RandString(6)
	clusterName := "tf-acc-plugin-" + suffix
	pluginName := "grafana"
	resourceName := "fundament_plugin_installation.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccPluginInstallationResourceConfig(clusterName, pluginName, endpoint, organizationID, kubeProxyURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "cluster_id"),
					resource.TestCheckResourceAttr(resourceName, "plugin_name", pluginName),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "phase", "Running"),
				),
			},
			// ImportState
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Delete is automatic at end of TestCase
		},
	})
}

func testAccPluginInstallationResourceConfig(clusterName, pluginName, endpoint, organizationID, kubeProxyURL string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint            = %[3]q
  organization_id     = %[4]q
  kube_api_proxy_url  = %[5]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = %[1]q
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_plugin_installation" "test" {
  cluster_id  = fundament_cluster.test.id
  plugin_name = %[2]q
}
`, clusterName, pluginName, endpoint, organizationID, kubeProxyURL)
}
