package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccProjectDataSource tests the fundament_project data source against a real API.
func TestAccProjectDataSource(t *testing.T) {
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

	projectID := os.Getenv("FUNDAMENT_TEST_PROJECT_ID")
	if projectID == "" {
		t.Skip("FUNDAMENT_TEST_PROJECT_ID not set, skipping project data source test")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectDataSourceConfig(projectID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fundament_project.test", "id", projectID),
					resource.TestCheckResourceAttrSet("data.fundament_project.test", "name"),
					resource.TestCheckResourceAttrSet("data.fundament_project.test", "created"),
				),
			},
		},
	})
}

func testAccProjectDataSourceConfig(projectID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
}

data "fundament_project" "test" {
  id = %[1]q
}
`, projectID)
}
