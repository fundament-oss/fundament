package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccClusterDataSource tests the fundament_cluster data source against a real API.
func TestAccClusterDataSource(t *testing.T) {
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
	clusterName := "tf-acc-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterDataSourceConfig(clusterName, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fundament_cluster.test", "name", clusterName),
					resource.TestCheckResourceAttrSet("data.fundament_cluster.test", "id"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster.test", "region"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster.test", "kubernetes_version"),
					resource.TestCheckResourceAttrSet("data.fundament_cluster.test", "status"),
				),
			},
		},
	})
}

func testAccClusterDataSourceConfig(clusterName, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[2]q
  organization_id = %[3]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = %[1]q
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

data "fundament_cluster" "test" {
  name = fundament_cluster.test.name
}
`, clusterName, endpoint, organizationID)
}
