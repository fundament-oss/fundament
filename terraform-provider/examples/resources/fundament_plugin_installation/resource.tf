# Needs kube_api_proxy_url in the provider configuration.
data "fundament_cluster" "example" {
  name = "my-cluster"
}

resource "fundament_plugin_installation" "example" {
  cluster_id        = data.fundament_cluster.example.id
  organization_name = "acme-corp"
  plugin_name       = "my-plugin"

  # Optional; without them the latest published version is pinned.
  # plugin_version  = "1.0.0"
  # definition_hash = "sha256:..."

  # Optional install-time configuration, checked against the plugin's configuration schema.
  # config = {
  #   ADMIN_USER = "admin"
  # }
}
