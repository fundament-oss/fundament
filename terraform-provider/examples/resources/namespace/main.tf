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

# Create a namespace in a cluster
resource "fundament_namespace" "example" {
  name       = "my-application"
  project_id = "your-project-uuid"
  cluster_id = "your-cluster-uuid"
}

output "namespace_id" {
  description = "The ID of the created namespace"
  value       = fundament_namespace.example.id
}

output "namespace_created" {
  description = "When the namespace was created"
  value       = fundament_namespace.example.created
}
