data "fundament_cluster" "example" {
  name = "my-cluster"
}

data "fundament_projects" "all" {
  cluster_id = data.fundament_cluster.example.id
}
