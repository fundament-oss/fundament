terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint = "http://organization.fundament.localhost:8080"
  # Token can be set via FUNDAMENT_TOKEN environment variable
  # token = ""
}

# Invite a member as admin
resource "fundament_organization_member" "admin" {
  email = "admin@example.com"
  role  = "admin"
}

# Invite a member as viewer
resource "fundament_organization_member" "viewer" {
  email = "viewer@example.com"
  role  = "viewer"
}

output "admin_member_id" {
  description = "The ID of the admin member"
  value       = fundament_organization_member.admin.id
}

output "viewer_member_id" {
  description = "The ID of the viewer member"
  value       = fundament_organization_member.viewer.id
}
