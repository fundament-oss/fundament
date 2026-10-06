resource "fundament_cluster" "example" {
  name               = "my-cluster"
  region             = "local"
  kubernetes_version = "1.33.0"
}
