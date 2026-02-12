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

# Create a project
resource "fundament_project" "example" {
  name = "my-project"
}

# Add a user as an admin member of the project
resource "fundament_project_member" "admin" {
  project_id = fundament_project.example.id
  user_id    = "<user1-id>"
  role       = "admin"
}

# Add another user as a viewer
resource "fundament_project_member" "viewer" {
  project_id = fundament_project.example.id
  user_id    = "<user2-id>"
  role       = "viewer"
}

output "admin_member_id" {
  description = "The member ID of the admin"
  value       = fundament_project_member.admin.id
}

output "admin_user_name" {
  description = "The name of the admin user"
  value       = fundament_project_member.admin.user_name
}
