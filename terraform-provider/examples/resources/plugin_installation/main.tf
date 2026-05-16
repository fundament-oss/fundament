terraform {
  required_providers {
    fundament = {
      source = "fundament-oss/fundament"
    }
  }
}

provider "fundament" {
  endpoint            = "https://organization.api.fundament.example.com"
  authn_endpoint      = "https://authn.api.fundament.example.com"
  api_key             = var.fundament_api_key
  organization_id     = var.organization_id
  kube_api_proxy_url  = "https://kube-api-proxy.fundament.example.com"
}

resource "fundament_cluster" "prod" {
  name               = "prod"
  region             = "eu-west-1"
  kubernetes_version = "1.30"
}

resource "fundament_plugin_installation" "grafana" {
  cluster_id  = fundament_cluster.prod.id
  plugin_name = "grafana"
  image       = "ghcr.io/fundament/grafana:v10.2.0"
}

# Import an existing plugin installation:
# terraform import fundament_plugin_installation.grafana <cluster-id>/grafana
