package provider

import (
	"fmt"
	"os"
	"regexp"
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

	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Two clusters each have a project with the name.
				Config:      testAccProjectDataSourceConfig(projectName, suffix, endpoint, organizationID, ""),
				ExpectError: regexp.MustCompile(`several clusters have a project`),
			},
			{
				Config: testAccProjectDataSourceConfig(projectName, suffix, endpoint, organizationID, "tf-acc-pds-b-"+suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fundament_project.test", "name", projectName),
					resource.TestCheckResourceAttrPair("data.fundament_project.test", "id", "fundament_project.b", "id"),
					resource.TestCheckResourceAttrPair("data.fundament_project.test", "cluster_id", "fundament_cluster.b", "id"),
					resource.TestCheckResourceAttr("data.fundament_project.test", "cluster_name", "tf-acc-pds-b-"+suffix),
					resource.TestMatchResourceAttr("data.fundament_project.test", "created", rfc3339),
				),
			},
		},
	})
}

func testAccProjectDataSourceConfig(projectName, suffix, endpoint, organizationID, clusterName string) string {
	clusterArg := ""
	if clusterName != "" {
		clusterArg = fmt.Sprintf("cluster_name = %q", clusterName)
	}
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[3]q
  organization_id = %[4]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "a" {
  name               = "tf-acc-pds-a-%[2]s"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_cluster" "b" {
  name               = "tf-acc-pds-b-%[2]s"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_project" "a" {
  name       = %[1]q
  cluster_id = fundament_cluster.a.id
}

resource "fundament_project" "b" {
  name       = %[1]q
  cluster_id = fundament_cluster.b.id
}

data "fundament_project" "test" {
  name       = %[1]q
  %[5]s
  depends_on = [fundament_project.a, fundament_project.b]
}
`, projectName, suffix, endpoint, organizationID, clusterArg)
}
