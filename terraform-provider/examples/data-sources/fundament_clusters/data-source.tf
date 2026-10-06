data "fundament_clusters" "all" {}

output "cluster_names" {
  value = [for c in data.fundament_clusters.all.clusters : c.name]
}
