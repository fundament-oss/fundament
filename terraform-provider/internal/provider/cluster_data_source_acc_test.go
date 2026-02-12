package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccClusterDataSource tests the fundament_cluster data source against a real API.
func TestAccClusterDataSource(t *testing.T) {
	// Skip if not running acceptance tests
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1 is set")
	}

	// Ensure required environment variables are set
	if os.Getenv("FUNDAMENT_ENDPOINT") == "" {
		t.Fatal("FUNDAMENT_ENDPOINT must be set for acceptance tests")
	}
	if os.Getenv("FUNDAMENT_TOKEN") == "" {
		t.Fatal("FUNDAMENT_TOKEN must be set for acceptance tests")
	}

	clusterName := os.Getenv("FUNDAMENT_TEST_CLUSTER_NAME")
	if clusterName == "" {
		t.Skip("FUNDAMENT_TEST_CLUSTER_NAME not set, skipping cluster data source test")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterDataSourceConfig(clusterName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fundament_cluster.test", "name", clusterName),
					resource.TestCheckResourceAttrSet("data.fundament_cluster.test", "id"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster.test", "region"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster.test", "kubernetes_version"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster.test", "status"),
				),
			},
		},
	})
}

func testAccClusterDataSourceConfig(clusterName string) string {
	return fmt.Sprintf(`
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
}

data "fundament_cluster" "test" {
  name = %[1]q
}
`, clusterName)
}
