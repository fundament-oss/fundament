---
title: OpenTofu provider
sidebar:
  order: 11
---

The Fundament provider lets you manage clusters, projects, namespaces and
members declaratively with [OpenTofu](https://opentofu.org/) or Terraform,
instead of clicking through the console or scripting the [CLI](./functl.md).

The [provider reference](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/index.md) describes every argument and
attribute; it is generated from the provider itself. This page is an
orientation.

## Requirements

- OpenTofu >= 1.11
- A running Fundament instance and an [API key](./api-keys.md)

## Install

The provider is not published to a registry. Prebuilt packages for Linux
(amd64, arm64), macOS (Apple Silicon) and Windows (amd64) are published to the
rolling `terraform-provider-latest` release, named so that OpenTofu finds them
in its local plugin directory without any CLI configuration:

```bash
# Pick your platform: linux_amd64, linux_arm64 or darwin_arm64
PLATFORM=linux_amd64
MIRROR=~/.terraform.d/plugins/registry.opentofu.org/fundament/fundament
mkdir -p "$MIRROR"
curl -fsSL -o "$MIRROR/terraform-provider-fundament_0.1.0_${PLATFORM}.zip" \
  "https://github.com/fundament-oss/fundament/releases/download/terraform-provider-latest/terraform-provider-fundament_0.1.0_${PLATFORM}.zip"
```

Keep the zip as downloaded: its name is how OpenTofu discovers the version
and platform. Windows, Terraform instead of OpenTofu, checksums and updating to
a newer build are covered in the
[provider README](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/README.md#installation).

## Configuration

```hcl
terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

# The API key comes from the FUNDAMENT_API_KEY environment variable.
provider "fundament" {
  endpoint        = "https://organization.fundament.example"
  organization_id = "019b4000-0000-7000-8000-000000000001"
}
```

| Argument | Description | Required |
| --- | --- | --- |
| `endpoint` | URL of the Fundament organization API; may also come from `FUNDAMENT_ENDPOINT` | Yes |
| `organization_id` | The organization to manage, shown in the console under **General**; may also come from `FUNDAMENT_ORGANIZATION_ID` | Yes |
| `api_key` | API key; may also come from `FUNDAMENT_API_KEY` | Yes |
| `authn_endpoint` | URL of the authentication API. Derived from `endpoint` when omitted; may also come from `FUNDAMENT_AUTHN_ENDPOINT` | No |
| `kube_api_proxy_url` | URL of the Kubernetes API proxy, only for `fundament_plugin_installation`; may also come from `FUNDAMENT_KUBE_API_PROXY_URL` | No |

Keep the key out of your configuration and state: pass it through
`FUNDAMENT_API_KEY` or a variable backed by your secret store. The provider
exchanges the API key for a short-lived token and refreshes it as needed.

## Resources

| Resource | Manages |
| --- | --- |
| [`fundament_cluster`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/resources/cluster.md) | A managed Kubernetes cluster. See [Clusters](./clusters.md) |
| [`fundament_project`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/resources/project.md) | A project on a cluster |
| [`fundament_namespace`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/resources/namespace.md) | A namespace of a project. See [Namespaces](./namespaces.md) |
| [`fundament_project_member`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/resources/project_member.md) | A user's membership of a project. See [Members and roles](./members-and-roles.md) |
| [`fundament_organization_member`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/resources/organization_member.md) | An invitation to the organization and the member's permission |
| [`fundament_plugin_installation`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/resources/plugin_installation.md) | A plugin installed on a cluster. See [Plugins](./plugins.md) |

## Data sources

| Data source | Reads |
| --- | --- |
| [`fundament_clusters`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/clusters.md) | All clusters in the organization |
| [`fundament_cluster`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/cluster.md) | A single cluster by name |
| [`fundament_cluster_namespaces`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/cluster_namespaces.md) | The namespaces in a cluster |
| [`fundament_projects`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/projects.md) | The projects on a cluster |
| [`fundament_project`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/project.md) | A single project by name |
| [`fundament_project_namespaces`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/project_namespaces.md) | The namespaces of a project |
| [`fundament_namespace`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/namespace.md) | A single namespace by cluster, project and name |
| [`fundament_project_members`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/project_members.md) | The members of a project |
| [`fundament_organization_members`](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/data-sources/organization_members.md) | The members of the organization |

## See also

- [Getting started](./getting-started.md): the same steps in the console.
- [API keys](./api-keys.md): creating the key the provider authenticates with.
