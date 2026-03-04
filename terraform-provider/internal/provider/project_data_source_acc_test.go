package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccProjectDataSource tests the fundament_project data source against a real API.
func TestAccProjectDataSource(t *testing.T) {
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
	projectName := fmt.Sprintf("tf-acc-pds-%s", suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectDataSourceConfig(projectName, suffix, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fundament_project.test", "name", projectName),
					resource.TestCheckResourceAttrSet("data.fundament_project.test", "id"),
					resource.TestCheckResourceAttrSet("data.fundament_project.test", "created"),
				),
			},
		},
	})
}

func testAccProjectDataSourceConfig(projectName, suffix, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[3]q
  organization_id = %[4]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = "tf-acc-pds-c-%[2]s"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_project" "test" {
  name       = %[1]q
  cluster_id = fundament_cluster.test.id
}

data "fundament_project" "test" {
  name       = %[1]q
  depends_on = [fundament_project.test]
}
`, projectName, suffix, endpoint, organizationID)
}
