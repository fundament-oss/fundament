package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccClusterResource_basic(t *testing.T) {
	// Skip if not running acceptance tests
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1 is set")
	}

	// Ensure required environment variables are set
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

	suffix := acctest.RandString(6)
	clusterName := "tf-acc-" + suffix
	resourceName := "fundament_cluster.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccClusterResourceConfig(clusterName, "1.28", endpoint, organizationID, testAccNodePool("workers", "n1-standard-1", 1, 3)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", clusterName),
					resource.TestCheckResourceAttr(resourceName, "region", "eu-west-1"),
					resource.TestCheckResourceAttr(resourceName, "kubernetes_version", "1.28"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.name", "workers"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.machine_type", "n1-standard-1"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.autoscale_min", "1"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.autoscale_max", "3"),
				),
			},
			// ImportState testing
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update kubernetes_version
			{
				Config: testAccClusterResourceConfig(clusterName, "1.29", endpoint, organizationID, testAccNodePool("workers", "n1-standard-1", 1, 3)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", clusterName),
					resource.TestCheckResourceAttr(resourceName, "region", "eu-west-1"),
					resource.TestCheckResourceAttr(resourceName, "kubernetes_version", "1.29"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
				),
			},
			// Resize a pool, change another's machine type and add a third
			{
				Config: testAccClusterResourceConfig(clusterName, "1.29", endpoint, organizationID,
					testAccNodePool("workers", "n1-standard-2", 2, 4)+testAccNodePool("extra", "n1-standard-1", 1, 1)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "node_pool.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.name", "workers"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.machine_type", "n1-standard-2"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.autoscale_min", "2"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.autoscale_max", "4"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.1.name", "extra"),
				),
			},
			// Remove a pool
			{
				Config: testAccClusterResourceConfig(clusterName, "1.29", endpoint, organizationID, testAccNodePool("extra", "n1-standard-1", 1, 1)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "node_pool.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "node_pool.0.name", "extra"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccNodePool(name, machineType string, autoscaleMin, autoscaleMax int) string {
	return fmt.Sprintf(`
  node_pool {
    name          = %q
    machine_type  = %q
    autoscale_min = %d
    autoscale_max = %d
  }
`, name, machineType, autoscaleMin, autoscaleMax)
}

func testAccClusterResourceConfig(name, kubernetesVersion, endpoint, organizationID, nodePools string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[3]q
  organization_id = %[4]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = %[1]q
  region             = "eu-west-1"
  kubernetes_version = %[2]q
%[5]s}
`, name, kubernetesVersion, endpoint, organizationID, nodePools)
}
