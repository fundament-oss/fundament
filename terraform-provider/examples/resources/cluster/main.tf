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
  # API Key can be set via FUNDAMENT_API_KEY environment variable
  # api_key = ""
}

# Create a Kubernetes cluster
resource "fundament_cluster" "example" {
  name               = "my-production-cluster"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

output "cluster_id" {
  description = "The ID of the created cluster"
  value       = fundament_cluster.example.id
}

output "cluster_status" {
  description = "The current status of the cluster"
  value       = fundament_cluster.example.status
}
