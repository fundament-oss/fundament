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
  endpoint        = "https://organization.fundament.localhost:10443"
  organization_id = "019b4000-0000-7000-8000-000000000002" # Globex
  # API Key can be set via FUNDAMENT_API_KEY environment variable
  # api_key = ""
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
