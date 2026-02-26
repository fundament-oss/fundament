package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccNamespaceDataSource tests the fundament_namespace data source against a real API.
func TestAccNamespaceDataSource(t *testing.T) {
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

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNamespaceDataSourceConfig(endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify the namespace attributes from the data source match the resource
					resource.TestCheckResourceAttrPair(
						"data.fundament_namespace.test", "id",
						"fundament_namespace.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.fundament_namespace.test", "name",
						"fundament_namespace.test", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.fundament_namespace.test", "project_id",
						"fundament_namespace.test", "project_id",
					),
					resource.TestCheckResourceAttrPair(
						"data.fundament_namespace.test", "cluster_id",
						"fundament_namespace.test", "cluster_id",
					),
					resource.TestCheckResourceAttrSet("data.fundament_namespace.test", "created"),
				),
			},
		},
	})
}

func testAccNamespaceDataSourceConfig(endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[1]q
  organization_id = %[2]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_project" "test" {
  name = "tf-acc-test-ns-ds-project"
}

resource "fundament_cluster" "test" {
  name               = "tf-acc-test-ns-ds-cluster"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_namespace" "test" {
  name       = "tf-acc-test-ns-ds"
  project_id = fundament_project.test.id
  cluster_id = fundament_cluster.test.id
}

data "fundament_namespace" "test" {
  cluster_name = fundament_cluster.test.name
  project_name = fundament_project.test.name
  name         = fundament_namespace.test.name
}
`, endpoint, organizationID)
}
