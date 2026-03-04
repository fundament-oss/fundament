package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProjectMembersDataSource(t *testing.T) {
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
				Config: testAccProjectMembersDataSourceConfig(suffix, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fundament_project_members.test", "project_id"),
					resource.TestCheckResourceAttrSet("data.fundament_project_members.test", "members.#"),
				),
			},
		},
	})
}

func testAccProjectMembersDataSourceConfig(suffix, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[2]q
  organization_id = %[3]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = "tf-acc-pmds-c-%[1]s"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_project" "test" {
  name       = "tf-acc-pmds-%[1]s"
  cluster_id = fundament_cluster.test.id
}

resource "fundament_organization_member" "test" {
  email      = "tf-acc-pmds-%[1]s@test.example.com"
  permission = "viewer"
}

resource "fundament_project_member" "test" {
  project_id = fundament_project.test.id
  user_id    = fundament_organization_member.test.user_id
  permission = "viewer"
}

data "fundament_project_members" "test" {
  project_id = fundament_project.test.id

  depends_on = [fundament_project_member.test]
}
`, suffix, endpoint, organizationID)
}
