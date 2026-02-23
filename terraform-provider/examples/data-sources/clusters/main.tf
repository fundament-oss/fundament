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
  authn_endpoint  = "http://authn.fundament.localhost:8080"
  # Token can be set via FUNDAMENT_TOKEN environment variable
  # token = ""
}

# List all clusters in the organization
data "fundament_clusters" "all" {}

output "data_source_id" {
  description = "The data source identifier"
  value       = data.fundament_clusters.all.id
}

output "all_clusters" {
  description = "All clusters in the organization"
  value       = data.fundament_clusters.all.clusters
}

output "cluster_names" {
  description = "Names of all clusters"
  value       = [for c in data.fundament_clusters.all.clusters : c.name]
}

# Example: Filter clusters by project (optional)
# data "fundament_clusters" "by_project" {
#   project_id = ""
# }
