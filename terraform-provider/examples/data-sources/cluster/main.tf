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

# Look up an existing cluster by ID
data "fundament_cluster" "example" {
  id = "your-cluster-uuid"
}

output "cluster_name" {
  description = "The name of the cluster"
  value       = data.fundament_cluster.example.name
}

output "cluster_status" {
  description = "The current status of the cluster"
  value       = data.fundament_cluster.example.status
}

output "kubernetes_version" {
  description = "The Kubernetes version of the cluster"
  value       = data.fundament_cluster.example.kubernetes_version
}
