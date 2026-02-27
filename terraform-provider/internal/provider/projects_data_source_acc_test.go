package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProjectsDataSource(t *testing.T) {
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

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectsDataSourceConfig(suffix, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify the data source ID is set
					resource.TestCheckResourceAttr("data.fundament_projects.test", "id", "projects"),
					// Verify projects attribute exists (may be empty list)
					resource.TestCheckResourceAttrSet("data.fundament_projects.test", "projects.#"),
				),
			},
		},
	})
}

func testAccProjectsDataSourceConfig(suffix, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[2]q
  organization_id = %[3]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = "tf-acc-pds-%[1]s"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

data "fundament_projects" "test" {
  cluster_id = fundament_cluster.test.id
}
`, suffix, endpoint, organizationID)
}
