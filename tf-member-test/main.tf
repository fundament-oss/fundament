terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint = "http://organization.fundament.localhost:8080"
  api_key  = "fun_t8wiAVJEVtpYFWD5ZTqwmVyqMdECRp0R8kbi"
}

# Globex test users (from seed data):
# David Brown  019b4000-1000-7000-8000-000000000004  (admin, owns the API key)
# Eve Davis    019b4000-1000-7000-8000-000000000005  (viewer)
locals {
  eve_email = "eve@globex.corp"
}

# Step 1: Invite Eve as a viewer to the organization
resource "fundament_organization_member" "eve" {
  email = local.eve_email
  role  = "viewer"
}

# Step 2: List all organization members
data "fundament_organization_members" "all" {
  depends_on = [fundament_organization_member.eve]
}

output "eve_member" {
  value = fundament_organization_member.eve
}

output "all_members" {
  value = data.fundament_organization_members.all.members
}
