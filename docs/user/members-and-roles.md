---
title: Members and roles
sidebar:
  order: 8
---

Access in Fundament is granted at three levels (organization, cluster and
project) and inherited downwards. This page covers who can do what and how to
manage it; the underlying resource model is described in
[Organizations and projects](./organizations.md).

## Levels

| Level | Roles | Scope |
| --- | --- | --- |
| Organization | Organization admin, member | All clusters and projects within the organization |
| Cluster | Cluster admin | Infrastructure management, all projects on the cluster |
| Project | Project admin, viewer | All namespaces within the project |

Users are invited to an organization first, and can then be added to individual
projects. Service accounts follow the same model.

## Managing organization members

**Organization → Members** lists everyone in the organization and their role.
Organization admins can invite new members, change roles and remove members.

**Organization → Settings** and **Organization → Limits** are also
admin-only; limits bound what the organization's clusters and projects may
consume in total.

## Managing project members

**Project → Members** controls who has access to a single project. A project
admin can manage members and settings for that project; a viewer has read-only
access.

**Project → Limits** bounds the resources the project's namespaces can consume,
within whatever the organization allows.

## Namespace role bindings

**Project → Roles** is where finer-grained access will live: binding a user to
Kubernetes roles within a specific namespace, so that someone can deploy in one
namespace while only reading in another.

:::note[Not yet in effect]
The page is a preview. It lets you build bindings out of four roles (`deploy`,
`view-pods`, `view-logs` and `manage-services`), but nothing is stored or
enforced yet, and the list of roles is not final.
:::

What is enforced today has no namespace dimension. A namespace inherits its
permissions wholesale from its project: a project admin can view, edit and
delete every namespace in the project, and a project viewer can view all of
them. On the cluster itself the split is coarser still: organization admins
get `cluster-admin`, everyone else gets whatever the cluster's own RBAC grants
their ServiceAccount. See
[Cluster access](./clusters.md#what-the-kubeconfig-grants).

Per-namespace role bindings derived from project membership are recorded as
future work in FUN-7, including the option of matching namespaces by pattern
(for example `staging-*`) rather than one binding at a time.

## Authorization model

Permissions are evaluated by OpenFGA using a relationship-based model, which is
what makes the inheritance above work: a grant at the organization level applies
to everything beneath it without having to be repeated per project. Plugins
integrate with the same model rather than defining their own.

## Non-interactive access

Scripts, CI pipelines and the [functl CLI](./functl.md) authenticate with
[API keys](./api-keys.md) rather than user credentials. An API key acts with the
permissions of the identity that created it.
