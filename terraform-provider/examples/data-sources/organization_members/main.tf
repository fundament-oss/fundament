terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

# David Brown is a Globex admin and owns the API key.
# This provider instance operates on the Globex organization.
provider "fundament" {
  endpoint        = "http://organization.fundament.localhost:8080"
  # Token can be set via FUNDAMENT_TOKEN environment variable
  # token = ""
  organization_id = "019b4000-0000-7000-8000-000000000002" # Globex
}

# List all Globex members
data "fundament_organization_members" "all" {}

output "member_count" {
  description = "The number of Globex members"
  value       = length(data.fundament_organization_members.all.members)
}

output "members" {
  description = "All Globex members"
  value       = data.fundament_organization_members.all.members
}
