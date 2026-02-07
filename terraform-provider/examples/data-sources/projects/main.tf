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

# List all projects in the organization
data "fundament_projects" "all" {}

output "data_source_id" {
  description = "The data source identifier"
  value       = data.fundament_projects.all.id
}

output "all_projects" {
  description = "All projects in the organization"
  value       = data.fundament_projects.all.projects
}

output "project_names" {
  description = "Names of all projects"
  value       = [for p in data.fundament_projects.all.projects : p.name]
}
