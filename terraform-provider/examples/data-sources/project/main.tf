terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint        = "https://organization.fundament.localhost:10443"
  organization_id = "019b4000-0000-7000-8000-000000000002" # Globex
  # API Key can be set via FUNDAMENT_API_KEY environment variable
  # api_key = ""
}

# Look up an existing project by name
data "fundament_project" "example" {
  name = "my-project"
}

output "project_id" {
  description = "The unique identifier of the project"
  value       = data.fundament_project.example.id
}

output "project_cluster_name" {
  description = "The name of the cluster this project belongs to"
  value       = data.fundament_project.example.cluster_name
}

output "project_created" {
  description = "The creation timestamp of the project"
  value       = data.fundament_project.example.created
}
