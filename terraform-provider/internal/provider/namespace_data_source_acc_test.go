package provider

import (
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
				Config: testAccNamespaceDataSourceConfig,
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
					resource.TestCheckResourceAttrSet("data.fundament_namespace.test", "created_at"),
				),
			},
		},
	})
}

const testAccNamespaceDataSourceConfig = `
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
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
  id         = fundament_namespace.test.id
  cluster_id = fundament_cluster.test.id
}
`
