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

# Create a namespace in a cluster
resource "fundament_namespace" "example" {
  name       = "my-application-1"
  project_name = "abcdef"
  cluster_name = "abc"
}

output "namespace_id" {
  description = "The ID of the created namespace"
  value       = fundament_namespace.example.id
}

output "namespace_created" {
  description = "When the namespace was created"
  value       = fundament_namespace.example.created
}
