---
title: OpenTofu provider
sidebar:
  order: 11
---

The Fundament provider lets you manage clusters, node pools, projects,
namespaces and members declaratively with [OpenTofu](https://opentofu.org/) or Terraform,
instead of clicking through the console or scripting the [CLI](./functl.md).

The [provider reference](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/index.md)
is generated from the provider itself: it lists every resource and data
source and describes every argument. Its
[complete example](https://github.com/fundament-oss/fundament/blob/master/terraform-provider/docs/index.md#complete-example)
creates a cluster with a node pool, a project, a namespace and a member in one
configuration. This page covers installing and configuring the provider.

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

The reference lists every provider argument and the environment variable it
can come from.

## See also

- [Getting started](./getting-started.md): the same steps in the console.
- [API keys](./api-keys.md): creating the key the provider authenticates with.
- [Clusters](./clusters.md), [Namespaces](./namespaces.md),
  [Members and roles](./members-and-roles.md) and [Plugins](./plugins.md): what
  the resources manage.
