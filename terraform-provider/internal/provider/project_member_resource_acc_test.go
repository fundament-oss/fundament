package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccProjectMemberResource_basic(t *testing.T) {
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
	resourceName := "fundament_project_member.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccProjectMemberResourceConfig("viewer", suffix, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "project_id"),
					resource.TestCheckResourceAttrSet(resourceName, "user_id"),
					resource.TestCheckResourceAttr(resourceName, "permission", "viewer"),
					resource.TestCheckResourceAttrSet(resourceName, "user_name"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
				),
			},
			// ImportState testing
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources[resourceName]
					if !ok {
						return "", fmt.Errorf("resource not found: %s", resourceName)
					}
					projectID := rs.Primary.Attributes["project_id"]
					memberID := rs.Primary.ID
					return fmt.Sprintf("%s:%s", projectID, memberID), nil
				},
			},
			// Update permission to admin
			{
				Config: testAccProjectMemberResourceConfig("admin", suffix, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "permission", "admin"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccProjectMemberResourceConfig(permission, suffix, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[3]q
  organization_id = %[4]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_cluster" "test" {
  name               = "tf-acc-pm-c-%[2]s"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

resource "fundament_project" "test" {
  name       = "tf-acc-pm-%[2]s"
  cluster_id = fundament_cluster.test.id
}

resource "fundament_organization_member" "test" {
  email      = "tf-acc-pm-%[2]s@test.example.com"
  permission = "viewer"
}

resource "fundament_project_member" "test" {
  project_id = fundament_project.test.id
  user_id    = fundament_organization_member.test.user_id
  permission = %[1]q
}
`, permission, suffix, endpoint, organizationID)
}
