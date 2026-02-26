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

// TestAccClustersDataSourceWithProjectFilter tests filtering clusters by project ID.
func TestAccClustersDataSourceWithProjectFilter(t *testing.T) {
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

	projectID := os.Getenv("FUNDAMENT_TEST_PROJECT_ID")
	if projectID == "" {
		t.Skip("FUNDAMENT_TEST_PROJECT_ID not set, skipping project filter test")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClustersDataSourceConfigWithProject(projectID, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify the data source ID includes project ID
					resource.TestCheckResourceAttr("data.fundament_clusters.by_project", "id", "clusters-"+projectID),
					resource.TestCheckResourceAttr("data.fundament_clusters.by_project", "project_id", projectID),
				),
			},
		},
	})
}

func testAccClustersDataSourceConfigWithProject(projectID, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[2]q
  organization_id = %[3]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

data "fundament_clusters" "by_project" {
  project_id = %[1]q
}
`, projectID, endpoint, organizationID)
}
