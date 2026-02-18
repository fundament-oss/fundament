package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccOrganizationMemberResource_basic(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1 is set")
	}

	if os.Getenv("FUNDAMENT_ENDPOINT") == "" {
		t.Fatal("FUNDAMENT_ENDPOINT must be set for acceptance tests")
	}
	if os.Getenv("FUNDAMENT_TOKEN") == "" {
		t.Fatal("FUNDAMENT_TOKEN must be set for acceptance tests")
	}

	testEmail := os.Getenv("FUNDAMENT_TEST_MEMBER_EMAIL")
	if testEmail == "" {
		t.Skip("FUNDAMENT_TEST_MEMBER_EMAIL not set, skipping organization member resource test")
	}

	resourceName := "fundament_organization_member.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with viewer role
			{
				Config: testAccOrganizationMemberResourceConfig(testEmail, "viewer"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "email", testEmail),
					resource.TestCheckResourceAttr(resourceName, "role", "viewer"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "name"),
					resource.TestCheckResourceAttrSet(resourceName, "created"),
				),
			},
			// ImportState testing
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update role to admin (in-place)
			{
				Config: testAccOrganizationMemberResourceConfig(testEmail, "admin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "email", testEmail),
					resource.TestCheckResourceAttr(resourceName, "role", "admin"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccOrganizationMemberResourceConfig(email, role string) string {
	return fmt.Sprintf(`
provider "fundament" {
  # Uses FUNDAMENT_ENDPOINT and FUNDAMENT_TOKEN from environment
}

resource "fundament_organization_member" "test" {
  email = %[1]q
  role  = %[2]q
}
`, email, role)
}
