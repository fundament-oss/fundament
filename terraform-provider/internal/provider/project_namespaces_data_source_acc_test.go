package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccProjectNamespacesDataSource tests the fundament_project_namespaces data source against a real API.
func TestAccProjectNamespacesDataSource(t *testing.T) {
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
				Config: testAccProjectNamespacesDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify the data source ID is set correctly
					resource.TestCheckResourceAttrPair(
						"data.fundament_project_namespaces.test", "id",
						"fundament_project.test", "id",
					),
					// Verify project_id matches
					resource.TestCheckResourceAttrPair(
						"data.fundament_project_namespaces.test", "project_id",
						"fundament_project.test", "id",
					),
					// Verify namespaces list exists and has at least 2 items
					resource.TestCheckResourceAttr("data.fundament_project_namespaces.test", "namespaces.#", "2"),
					// Verify first namespace attributes
					resource.TestCheckResourceAttrSet("data.fundament_project_namespaces.test", "namespaces.0.id"),
					resource.TestCheckResourceAttrSet("data.fundament_project_namespaces.test", "namespaces.0.name"),
					resource.TestCheckResourceAttrSet("data.fundament_project_namespaces.test", "namespaces.0.cluster_id"),
					resource.TestCheckResourceAttrSet("data.fundament_project_namespaces.test", "namespaces.0.created_at"),
				),
			},
		},
	})
}

const testAccProjectNamespacesDataSourceConfig = `
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
}

resource "fundament_project" "test" {
  name = "tf-acc-test-project-ns"
}

resource "fundament_cluster" "test1" {
  name               = "tf-acc-test-project-ns-cluster-1"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_cluster" "test2" {
  name               = "tf-acc-test-project-ns-cluster-2"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_namespace" "test1" {
  name       = "tf-acc-test-project-ns-1"
  project_id = fundament_project.test.id
  cluster_id = fundament_cluster.test1.id
}

resource "fundament_namespace" "test2" {
  name       = "tf-acc-test-project-ns-2"
  project_id = fundament_project.test.id
  cluster_id = fundament_cluster.test2.id
}

data "fundament_project_namespaces" "test" {
  project_id = fundament_project.test.id
  depends_on = [fundament_namespace.test1, fundament_namespace.test2]
}
`
