terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

# A cluster with one node pool. Region, Kubernetes version and machine type
# must be ones the installation offers; the console's new-cluster form lists them.
resource "fundament_cluster" "demo" {
  name               = "demo"
  region             = "local"
  kubernetes_version = "1.33.0"

  node_pool {
    name          = "workers"
    machine_type  = "local-small"
    autoscale_min = 1
    autoscale_max = 3
  }
}

resource "fundament_project" "demo" {
  name       = "demo"
  cluster_id = fundament_cluster.demo.id
}

# In the cluster: tnt-demo--web.
resource "fundament_namespace" "web" {
  name       = "web"
  project_id = fundament_project.demo.id
}

data "fundament_organization_members" "all" {}

resource "fundament_project_member" "viewer" {
  project_id = fundament_project.demo.id
  user_id    = one([for m in data.fundament_organization_members.all.members : m.user_id if m.email == "colleague@example.com"])
  permission = "viewer"
}
