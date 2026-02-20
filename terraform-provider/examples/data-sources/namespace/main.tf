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

# Look up an existing namespace by cluster name and namespace name
data "fundament_namespace" "by_cluster" {
  cluster_name = "production"
  name         = "my-namespace"
}

# Or look up a namespace by project name and namespace name
data "fundament_namespace" "by_project" {
  project_name = "my-project"
  name         = "my-namespace"
}

output "namespace_id" {
  description = "The unique identifier of the namespace"
  value       = data.fundament_namespace.by_cluster.id
}

output "namespace_project_id" {
  description = "The project ID that owns this namespace"
  value       = data.fundament_namespace.by_cluster.project_id
}

output "namespace_cluster_id" {
  description = "The cluster ID where this namespace is deployed"
  value       = data.fundament_namespace.by_cluster.cluster_id
}

output "namespace_created" {
  description = "When the namespace was created"
  value       = data.fundament_namespace.by_cluster.created
}
