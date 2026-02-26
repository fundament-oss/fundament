package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccProjectMemberResource_basic tests basic project member CRUD operations.
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

	userID := os.Getenv("FUNDAMENT_TEST_USER_ID")
	if userID == "" {
		t.Fatal("FUNDAMENT_TEST_USER_ID must be set for project member acceptance tests")
	}

	suffix := acctest.RandString(6)
	resourceName := "fundament_project_member.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccProjectMemberResourceConfig(userID, "viewer", suffix, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "project_id"),
					resource.TestCheckResourceAttr(resourceName, "user_id", userID),
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
						return "", fmt.Errorf("Resource not found: %s", resourceName)
					}
					projectID := rs.Primary.Attributes["project_id"]
					memberID := rs.Primary.ID
					return fmt.Sprintf("%s:%s", projectID, memberID), nil
				},
			},
			// Update permission
			{
				Config: testAccProjectMemberResourceConfig(userID, "admin", suffix, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "permission", "admin"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccProjectMemberResourceConfig(userID, permission, suffix, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[4]q
  organization_id = %[5]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_project" "test" {
  name = "tf-acc-pm-%[3]s"
}

resource "fundament_project_member" "test" {
  project_id = fundament_project.test.id
  user_id    = %[1]q
  permission = %[2]q
}
`, userID, permission, suffix, endpoint, organizationID)
}
