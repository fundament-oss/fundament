package provider

import (
	"fmt"
	"os"
	"testing"

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
	if os.Getenv("FUNDAMENT_ENDPOINT") == "" {
		t.Fatal("FUNDAMENT_ENDPOINT must be set for acceptance tests")
	}
	if os.Getenv("FUNDAMENT_TOKEN") == "" && os.Getenv("FUNDAMENT_API_KEY") == "" {
		t.Fatal("FUNDAMENT_TOKEN or FUNDAMENT_API_KEY must be set for acceptance tests")
	}

	userID := os.Getenv("FUNDAMENT_TEST_USER_ID")
	if userID == "" {
		t.Fatal("FUNDAMENT_TEST_USER_ID must be set for project member acceptance tests")
	}

	resourceName := "fundament_project_member.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccProjectMemberResourceConfig(userID, "viewer"),
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
				Config: testAccProjectMemberResourceConfig(userID, "admin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "permission", "admin"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccProjectMemberResourceConfig(userID, permission string) string {
	return fmt.Sprintf(`
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN or FUNDAMENT_API_KEY from environment
}

resource "fundament_project" "test" {
  name = "tf-acc-test-member-project"
}

resource "fundament_project_member" "test" {
  project_id = fundament_project.test.id
  user_id    = %[1]q
  permission = %[2]q
}
`, userID, permission)
}
