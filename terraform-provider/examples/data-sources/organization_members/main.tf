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

# List all organization members
data "fundament_organization_members" "all" {}

output "member_count" {
  description = "The number of organization members"
  value       = length(data.fundament_organization_members.all.members)
}

output "members" {
  description = "All organization members"
  value       = data.fundament_organization_members.all.members
}
