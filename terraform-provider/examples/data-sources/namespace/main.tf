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

# Look up an existing namespace by ID
data "fundament_namespace" "example" {
  id         = "your-namespace-uuid"
  cluster_id = "your-cluster-uuid"
}

output "namespace_name" {
  description = "The name of the namespace"
  value       = data.fundament_namespace.example.name
}

output "namespace_project" {
  description = "The project ID that owns this namespace"
  value       = data.fundament_namespace.example.project_id
}

output "namespace_created" {
  description = "When the namespace was created"
  value       = data.fundament_namespace.example.created
}
