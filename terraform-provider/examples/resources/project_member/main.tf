terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint = "http://organization.fundament.localhost:8080"
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

# Add Eve Davis as an admin member of the project
resource "fundament_project_member" "admin" {
  project_id = fundament_project.example.id
  user_id    = "019b4000-1000-7000-8000-000000000005" # Eve Davis
  permission = "admin"
}

output "admin_member_id" {
  description = "The member ID of the admin"
  value       = fundament_project_member.admin.id
}

output "admin_user_name" {
  description = "The name of the admin user"
  value       = fundament_project_member.admin.user_name
}
