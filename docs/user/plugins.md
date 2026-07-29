---
title: Plugins
sidebar:
  order: 7
---

In Fundament, popular platform capabilities such as storage, networking and database services are provided as Plugins. Each Plugin is installed on a per-cluster basis, giving organizations control over which features they want to enable in their environment. Rather than reinventing these services, Plugins often wrap proven open source projects -preferably CNCF projects- to deliver 𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒-grade functionality in a Kubernetes-native way.

For example, a CloudSQL-like Plugin could be offered via Postgres or MySQL operators, an Application Load Balancer Plugin could build on Ingress controllers like Envoy Gateway, and S3-compatible object storage could be powered by projects such as MinIO or Ceph RADOS Gateway. Similarly, Block Storage can be integrated through the Container Storage Interface (CSI) with backends like Rook/Ceph. By exposing these services as Plugins, Fundament ensures tenants can assemble the platform they need, combining familiar cloud features with the transparency and flexibility of open source components.

Organizations can also build and install their own Plugins. This allows them to experiment with new technologies without having to wait for external parties to catch up. Additionally, organizations can contribute their own Plugins back to the Fundament community, helping to build a rich ecosystem of plugins for the platform.

If you want to build a Plugin yourself, see the [Plugin development](/docs/developer/plugins) documentation.

## Installation and versioning

A Plugin is installed within a Cluster. Each Cluster can have a different set of Plugins installed, and each Cluster can have a different version of a plugin installed.

A Plugin is installed as a Helm Chart, with optional additional configuration and customization overlays.

## Plugin Marketplace

The [Plugin Marketplace](https://console.fundament.projects.digilab.network/plugins) allows Cluster Admins to find and install Plugins into their Cluster.

There are four labels of Plugins. These indicate the quality and level of support of a Plugin:

_Terms/names to be refined._

- Core: Provided and maintained by the Fundament team.
- Rijksoverheid: Published by a Dutch government organization and validated by the Fundament team, but operated and maintained by the publishing organization itself. Suitable for production use within the public sector.
- 9-to-17 support: Published by a verified Team, with support only guaranteed during Dutch office hours (workdays, 9:00-17:00). Outside those hours no response is guaranteed, so do not rely on it for workloads that need round-the-clock support.
- Sideloaded: The plugin is not available in the Plugin Marketplace and can only be used within the organization that has developed it. Other organizations can install it manually if they put their cluster in Plugin Development Mode.
