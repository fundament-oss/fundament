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

# Create a project using cluster name
resource "fundament_project" "example" {
  cluster_name = "my-cluster"
  name         = "my-production-project"
}

# Or create a project using cluster ID
resource "fundament_project" "example_by_id" {
  cluster_id = "01234567-89ab-cdef-0123-456789abcdef"
  name       = "my-other-project"
}

output "project_id" {
  description = "The ID of the created project"
  value       = fundament_project.example.id
}

output "project_cluster_name" {
  description = "The cluster name (computed when using cluster_id)"
  value       = fundament_project.example.cluster_name
}

output "project_created" {
  description = "The creation timestamp of the project"
  value       = fundament_project.example.created
}
