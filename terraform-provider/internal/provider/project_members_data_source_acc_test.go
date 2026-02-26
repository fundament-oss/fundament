package provider

import (
	"fmt"
	"os"
	"testing"

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

	userID := os.Getenv("FUNDAMENT_TEST_USER_ID")
	if userID == "" {
		t.Fatal("FUNDAMENT_TEST_USER_ID must be set for project member acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectMembersDataSourceConfig(userID, endpoint, organizationID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fundament_project_members.test", "project_id"),
					resource.TestCheckResourceAttrSet("data.fundament_project_members.test", "members.#"),
				),
			},
		},
	})
}

func testAccProjectMembersDataSourceConfig(userID, endpoint, organizationID string) string {
	return fmt.Sprintf(`
provider "fundament" {
  endpoint        = %[2]q
  organization_id = %[3]q
  # api_key read from environment variable FUNDAMENT_API_KEY
}

resource "fundament_project" "test" {
  name = "tf-acc-test-members-ds-project"
}

resource "fundament_project_member" "test" {
  project_id = fundament_project.test.id
  user_id    = %[1]q
  permission = "viewer"
}

data "fundament_project_members" "test" {
  project_id = fundament_project.test.id

  depends_on = [fundament_project_member.test]
}
`, userID, endpoint, organizationID)
}
