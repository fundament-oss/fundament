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
				Config: testAccClusterResourceConfig(clusterName, "eu-west-1", "1.28", endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", clusterName),
					resource.TestCheckResourceAttr(resourceName, "region", "eu-west-1"),
					resource.TestCheckResourceAttr(resourceName, "kubernetes_version", "1.28"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
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
				Config: testAccClusterResourceConfig(clusterName, "eu-west-1", "1.29", endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", clusterName),
					resource.TestCheckResourceAttr(resourceName, "region", "eu-west-1"),
					resource.TestCheckResourceAttr(resourceName, "kubernetes_version", "1.29"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccClusterResourceConfig(name, region, kubernetesVersion, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[4]q
  organization_id = %[5]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = %[1]q
  region             = %[2]q
  kubernetes_version = %[3]q
}
`, name, region, kubernetesVersion, endpoint, organizationID)
}
