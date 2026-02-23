terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint        = "http://organization.fundament.localhost:8080"
  organization_id = "019b4000-0000-7000-8000-000000000002" # Globex
  # API key can be set via FUNDAMENT_API_KEY environment variable
  # api_key = ""
}

# This example assumes you are authenticated as David Brown (admin) at the Globex organization.
# Globex seed users:
#   David Brown  019b4000-1000-7000-8000-000000000004  (admin)
#   Eve Davis    019b4000-1000-7000-8000-000000000005  (viewer)

# Create a project (David becomes implicit admin member)
resource "fundament_project" "example" {
  name = "my-project"
}

# Add Eve Davis as a viewer member of the project
resource "fundament_project_member" "example" {
  project_id = fundament_project.example.id
  user_id    = "019b4000-1000-7000-8000-000000000005" # Eve Davis
  permission = "viewer"
}

# List all members of the project
data "fundament_project_members" "all" {
  project_id = fundament_project.example.id

  depends_on = [fundament_project_member.example]
}

output "project_members" {
  description = "All members of the project"
  value       = data.fundament_project_members.all.members
}
