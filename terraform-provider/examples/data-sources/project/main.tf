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
  # Token can be set via FUNDAMENT_TOKEN environment variable
  # token = ""
}

# Look up an existing project by name
data "fundament_project" "example" {
  name = "my-project"
}

output "project_id" {
  description = "The unique identifier of the project"
  value       = data.fundament_project.example.id
}

output "project_created" {
  description = "The creation timestamp of the project"
  value       = data.fundament_project.example.created
}
