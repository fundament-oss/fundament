resource "fundament_cluster" "example" {
  name               = "my-cluster"
  region             = "local"
  kubernetes_version = "1.33.0"

  node_pool {
    name          = "workers"
    machine_type  = "local-small"
    autoscale_min = 1
    autoscale_max = 3
  }
}
