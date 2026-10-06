data "fundament_cluster" "example" {
  name = "my-cluster"
}

data "fundament_cluster_namespaces" "all" {
  cluster_id = data.fundament_cluster.example.id
}
