---
title: Tools
sidebar:
  order: 4
---

In Fundament, popular platform capabilities such as storage, networking and database services are provided as Tools. Each Tool is installed on a per-cluster basis, giving organizations control over which features they want to enable in their environment. Rather than reinventing these services, Tools often wrap proven open source projects -preferably CNCF projects- to deliver 𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒-grade functionality in a Kubernetes-native way.

For example, a CloudSQL-like Tool could be offered via Postgres or MySQL operators, an Application Load Balancer Tool could build on Ingress controllers like Envoy Gateway, and S3-compatible object storage could be powered by projects such as MinIO or Ceph RADOS Gateway. Similarly, Block Storage can be integrated through the Container Storage Interface (CSI) with backends like Rook/Ceph. By exposing these services as Tools, Fundament ensures tenants can assemble the platform they need, combining familiar cloud features with the transparency and flexibility of open source components.

Organizations can also build and install their own Tools. This allows them to experiment with new technologies without having to wait for external parties to catch up. Additionally, organizations can contribute their own Tools back to the Fundament community, helping to build a rich ecosystem of tools for the platform.

## Installation and versioning

A Tool is installed within a Cluster. Each Cluster can have a different set of Tools installed, and each Cluster can have a different version of a tool installed.

A Tool is installed as a Helm Chart, with optional additional configuration and customization overlays.

## Tool Catalog

The Tool Catalog allows Cluster Admins to find and install Tools into their Cluster.

There are four tiers of Tools. These indicate the quality and level of support of a Tool:

_Terms/names to be refined._

- Gold / Built-in: Provided and maintained by the Fundament team.
- Silver / Certified: Validated by the Fundament team, operated by the plugin developer.
- Bronze / Experimental: The plugin itself is not checked, but the publishing Team was verified. Not endorsed by Fundament but allowed to publish the plugin in the plugin catalog. Comes with a big warning.
- Grey / Internal: The plugin is not available in the plugin catalog and can only be used within the organization that has developed it. Other organizations can install it manually if they put their cluster in Tool Development Mode.
