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
  token = "fun_kL00uHi7yqkeSlfOmKu8NmpzUMOy1u3a3gj2"
}

# Reference an existing project
resource "fundament_project" "example" {
  name = "my-project"
}

# List all members of the project
data "fundament_project_members" "all" {
  project_id = fundament_project.example.id
}

output "project_members" {
  description = "All members of the project"
  value       = data.fundament_project_members.all.members
}
