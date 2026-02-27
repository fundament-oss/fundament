---
title: Organizations
sidebar:
  order: 3
---

# Resource Model

Fundament organises resources in a hierarchy. An organization owns clusters and projects. Each project runs on exactly one cluster, but a cluster can host multiple projects.

![Resource Model](assets/resource-model.drawio.svg)

## Organization

The top-level entity. Represents a legal entity, government body, or company that uses the Fundament platform. An organization owns all clusters, projects, and users beneath it.

Users can belong to multiple organizations. Membership and roles are managed per organization.

## Cluster

A Kubernetes cluster managed by Gardener, running on bare-metal machines provisioned by metal-stack. Clusters are infrastructure: they provide compute, storage, and networking.

A cluster belongs to one organization. An organization can have multiple clusters (e.g. production, staging, different regions). A single cluster can host multiple projects.

## Project

A logical grouping for ownership and access control. A project represents a team, application, or workload group within a specific environment. Project members share access to the project's namespaces.

A project belongs to one organization and runs on exactly one cluster. A common pattern is to create separate projects per environment:

- `webshop-prod` on the production cluster
- `webshop-test` on the test cluster

The project defines who has access and what the namespaces are for. The cluster defines where they run.

## Namespace

A Kubernetes namespace within a project. Since a project runs on one cluster, the namespace's location is determined by its project.

- Resource quotas and limit ranges are applied per namespace
- Network policies scope traffic within and between namespaces
- RBAC is scoped to the namespace level

## Example

The diagram above shows two organizations. Acme Corp has a production and a test cluster, each with separate projects per application. Globex Inc has a single production cluster with two projects. Each project contains namespaces for its workloads.

## Identity & Access

Access control follows the resource hierarchy:

| Level | Roles | Scope |
|---|---|---|
| Organization | Organization admin, member | All clusters and projects within the organization |
| Cluster | Cluster admin | Infrastructure management, all projects on the cluster |
| Project | Project admin, viewer | All namespaces within the project |

Users are invited to an organization and can then be added to projects. Service accounts follow the same model.

## Tools

[Tools](./tools.md) are installed per cluster. All projects on a cluster have access to the tools installed on that cluster. See [Tools](./tools.md) for details.
