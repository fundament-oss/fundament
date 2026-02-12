package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccProjectMembersDataSource tests the fundament_project_members data source against a real API.
// Set TF_ACC=1 and configure FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN to run.
func TestAccProjectMembersDataSource(t *testing.T) {
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

	userID := os.Getenv("FUNDAMENT_TEST_USER_ID")
	if userID == "" {
		t.Fatal("FUNDAMENT_TEST_USER_ID must be set for project member acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectMembersDataSourceConfig(userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fundament_project_members.test", "project_id"),
					resource.TestCheckResourceAttrSet("data.fundament_project_members.test", "members.#"),
				),
			},
		},
	})
}

func testAccProjectMembersDataSourceConfig(userID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
}

resource "fundament_project" "test" {
  name = "tf-acc-test-members-ds-project"
}

resource "fundament_project_member" "test" {
  project_id = fundament_project.test.id
  user_id    = %[1]q
  role       = "viewer"
}

data "fundament_project_members" "test" {
  project_id = fundament_project.test.id

  depends_on = [fundament_project_member.test]
}
`, userID)
}
