package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccClusterNamespacesDataSource(t *testing.T) {
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
				Config: testAccClusterNamespacesDataSourceConfig(endpoint, organizationID, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify the data source ID is set correctly
					resource.TestCheckResourceAttrPair(
						"data.fundament_cluster_namespaces.test", "id",
						"fundament_cluster.test", "id",
					),
					// Verify cluster_id matches
					resource.TestCheckResourceAttrPair(
						"data.fundament_cluster_namespaces.test", "cluster_id",
						"fundament_cluster.test", "id",
					),
					// Verify namespaces list exists and has at least 2 items
					resource.TestCheckResourceAttr("data.fundament_cluster_namespaces.test", "namespaces.#", "2"),
					// Verify first namespace attributes
					resource.TestCheckResourceAttrSet("data.fundament_cluster_namespaces.test", "namespaces.0.id"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster_namespaces.test", "namespaces.0.name"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster_namespaces.test", "namespaces.0.project_id"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster_namespaces.test", "namespaces.0.created"),
				),
			},
		},
	})
}

func testAccClusterNamespacesDataSourceConfig(endpoint, organizationID, suffix string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[1]q
  organization_id = %[2]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = "tf-acc-cn-%[3]s"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_project" "test" {
  name       = "tf-acc-cn-p-%[3]s"
  cluster_id = fundament_cluster.test.id
}

resource "fundament_namespace" "test1" {
  name       = "tf-acc-cn-1-%[3]s"
  project_id = fundament_project.test.id
  cluster_id = fundament_cluster.test.id
}

resource "fundament_namespace" "test2" {
  name       = "tf-acc-cn-2-%[3]s"
  project_id = fundament_project.test.id
  cluster_id = fundament_cluster.test.id
}

data "fundament_cluster_namespaces" "test" {
  cluster_id = fundament_cluster.test.id
  depends_on = [fundament_namespace.test1, fundament_namespace.test2]
}
`, endpoint, organizationID, suffix)
}
