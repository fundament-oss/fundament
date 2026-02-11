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

# List all namespaces in a cluster
data "fundament_cluster_namespaces" "all" {
  cluster_id = "your-cluster-uuid"
}

output "data_source_id" {
  description = "The data source identifier"
  value       = data.fundament_cluster_namespaces.all.id
}

output "all_namespaces" {
  description = "All namespaces in the cluster"
  value       = data.fundament_cluster_namespaces.all.namespaces
}

output "namespace_names" {
  description = "Names of all namespaces in the cluster"
  value       = [for ns in data.fundament_cluster_namespaces.all.namespaces : ns.name]
}

output "namespace_projects" {
  description = "Project IDs of all namespaces"
  value       = [for ns in data.fundament_cluster_namespaces.all.namespaces : ns.project_id]
}
