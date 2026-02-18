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

# Invite Alice (an Acme user) to join Globex as a viewer
resource "fundament_organization_member" "alice" {
  email = "alice@acme.corp"
  role  = "viewer"
}

output "alice_member" {
  description = "The invited member"
  value       = fundament_organization_member.alice
}
