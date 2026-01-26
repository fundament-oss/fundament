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
