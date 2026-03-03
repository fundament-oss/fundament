package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccClustersDataSource(t *testing.T) {
	// Skip if not running acceptance tests
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1 is set")
	}

	// Ensure required environment variables are set
	if os.Getenv("FUNDAMENT_API_KEY") == "" {
		t.Fatal("FUNDAMENT_API_KEY must be set for acceptance tests")
	}
	if os.Getenv("FUNDAMENT_ENDPOINT") == "" {
		t.Fatal("FUNDAMENT_ENDPOINT must be set for acceptance tests")
	}
	if os.Getenv("FUNDAMENT_ORGANIZATION_ID") == "" {
		t.Fatal("FUNDAMENT_ORGANIZATION_ID must be set for acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClustersDataSourceConfig(os.Getenv("FUNDAMENT_ENDPOINT"), os.Getenv("FUNDAMENT_ORGANIZATION_ID")),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify the data source ID is set
					resource.TestCheckResourceAttr("data.fundament_clusters.test", "id", "clusters"),
					// Verify clusters attribute exists (may be empty list)
					resource.TestCheckResourceAttrSet("data.fundament_clusters.test", "clusters.#"),
				),
			},
		},
	})
}

func testAccClustersDataSourceConfig(endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[1]q
  organization_id = %[2]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

data "fundament_clusters" "test" {}
`, endpoint, organizationID)
}
