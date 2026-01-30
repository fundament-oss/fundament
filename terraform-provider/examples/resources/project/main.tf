terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint = "http://organization.127.0.0.1.nip.io:8080"
  # Token can be set via FUNDAMENT_TOKEN environment variable
  # token = ""
}

# Create a project
resource "fundament_project" "example" {
  name = "my-production-project"
}

output "project_id" {
  description = "The ID of the created project"
  value       = fundament_project.example.id
}

output "project_created_at" {
  description = "The creation timestamp of the project"
  value       = fundament_project.example.created_at
}
