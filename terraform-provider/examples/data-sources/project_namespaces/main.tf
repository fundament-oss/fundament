terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint        = "http://organization.fundament.localhost:8080"
  organization_id = "019b4000-0000-7000-8000-000000000001" # Globex
  # Token can be set via FUNDAMENT_TOKEN environment variable
  # token = ""
}

# List all namespaces belonging to a project
data "fundament_project_namespaces" "all" {
  project_id = "019c8ea3-804d-734b-a31d-4bf074fc578c"
}

output "data_source_id" {
  description = "The data source identifier"
  value       = data.fundament_project_namespaces.all.id
}

output "all_namespaces" {
  description = "All namespaces in the project"
  value       = data.fundament_project_namespaces.all.namespaces
}

output "namespace_names" {
  description = "Names of all namespaces in the project"
  value       = [for ns in data.fundament_project_namespaces.all.namespaces : ns.name]
}

# Example: Combine with project resource
resource "fundament_project" "example" {
  cluster_name = "abc"
  name = "abcdefgejasdfsa"
}

data "fundament_project_namespaces" "example_project" {
  project_id = fundament_project.example.id
}

output "example_project_namespace_count" {
  description = "Number of namespaces in the example project"
  value       = length(data.fundament_project_namespaces.example_project.namespaces)
}
