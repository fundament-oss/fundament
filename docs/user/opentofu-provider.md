---
title: OpenTofu provider
sidebar:
  order: 11
---

The Fundament provider lets you manage clusters and project members
declaratively with [OpenTofu](https://opentofu.org/) or Terraform, instead of
clicking through the console or scripting the [CLI](./functl.md).

The
[provider README](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/README.md)
is the canonical reference for every argument and attribute; this page is an
orientation.

## Requirements

- OpenTofu >= 1.11
- A running Fundament instance and an [API key](./api-keys.md)

## Configuration

```hcl
terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint = "https://organization-api.example"
  api_key  = var.fundament_api_key # or set FUNDAMENT_API_KEY
}
```

| Argument | Description | Required |
| --- | --- | --- |
| `endpoint` | URL of the Fundament organization API | Yes |
| `api_key` | API key; may also come from `FUNDAMENT_API_KEY` | Yes |
| `authn_endpoint` | URL of the authentication API. Derived from `endpoint` when omitted; may also come from `FUNDAMENT_AUTHN_ENDPOINT` | No |

Keep the key out of your configuration and state: pass it through
`FUNDAMENT_API_KEY` or a variable backed by your secret store. The provider
exchanges the API key for a short-lived token and refreshes it as needed.

## Resources

| Resource | Manages |
| --- | --- |
| `fundament_cluster` | A managed Kubernetes cluster — see [Clusters](./clusters.md) |
| `fundament_project_member` | A user's membership of a project — see [Members and roles](./members-and-roles.md) |

## Data sources

| Data source | Reads |
| --- | --- |
| `fundament_clusters` | All clusters in the organization, optionally filtered by project |
| `fundament_cluster` | A single cluster by ID |
| `fundament_project_members` | The members of a project |

## Example

```hcl
data "fundament_clusters" "all" {}

output "cluster_names" {
  value = [for c in data.fundament_clusters.all.clusters : c.name]
}
```

## See also

- [Getting started](./getting-started.md) — the same steps in the console.
- [API keys](./api-keys.md) — creating the key the provider authenticates with.
