package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccClustersDataSource tests the fundament_clusters data source against a real API.
// Set TF_ACC=1 and configure FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN to run.
func TestAccClustersDataSource(t *testing.T) {
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
				Config: testAccClustersDataSourceConfig,
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

const testAccClustersDataSourceConfig = `
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
}

data "fundament_clusters" "test" {}
`

// TestAccClustersDataSourceWithProjectFilter tests filtering clusters by project ID.
func TestAccClustersDataSourceWithProjectFilter(t *testing.T) {
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
		t.Skip("FUNDAMENT_TEST_PROJECT_ID not set, skipping project filter test")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClustersDataSourceConfigWithProject(projectID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify the data source ID includes project ID
					resource.TestCheckResourceAttr("data.fundament_clusters.by_project", "id", "clusters-"+projectID),
					resource.TestCheckResourceAttr("data.fundament_clusters.by_project", "project_id", projectID),
				),
			},
		},
	})
}

func testAccClustersDataSourceConfigWithProject(projectID string) string {
	return `
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
}

data "fundament_clusters" "by_project" {
  project_id = "` + projectID + `"
}
`
}
