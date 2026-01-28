package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccProjectsDataSource tests the fundament_projects data source against a real API.
// Set TF_ACC=1 and configure FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN to run.
func TestAccProjectsDataSource(t *testing.T) {
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

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectsDataSourceConfig,
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

const testAccProjectsDataSourceConfig = `
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
}

data "fundament_projects" "test" {}
`
