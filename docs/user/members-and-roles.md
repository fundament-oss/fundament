---
title: Members and roles
sidebar:
  order: 9
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

**Organization → Settings** is also admin-only, as is a cluster's **Defaults**
block: an organization admin sets the resource requests and limits containers on
that cluster get for each request or limit they leave unset.

## Managing project members

**Project → Members** controls who has access to a single project. A project
admin can manage members and settings for that project; a viewer has read-only
access.

A project admin sets the project's own **Defaults**, on **Project → General**,
within whatever the cluster allows. See [Defaults](./namespaces.md#defaults).

## Namespaces

Access has no namespace dimension. A namespace inherits its permissions
wholesale from its project: a project admin can view, edit and delete every
namespace in the project, and a project viewer can view all of them. On the
cluster the same split applies: organization admins get `cluster-admin`;
project admins are bound to the built-in `admin` role and project viewers to
the built-in `view` role in every namespace of their project. See
[Cluster access](./clusters.md#what-the-kubeconfig-grants).

There is no way to give someone access to a single namespace. If workloads
need to be separated by who can reach them, put them in separate projects.

## Authorization model

Permissions are evaluated by OpenFGA using a relationship-based model, which is
what makes the inheritance above work: a grant at the organization level applies
to everything beneath it without having to be repeated per project. Plugins
integrate with the same model rather than defining their own.

## Non-interactive access

Scripts, CI pipelines and the [functl CLI](./functl.md) authenticate with
[API keys](./api-keys.md) rather than user credentials. An API key acts with the
permissions of the identity that created it.
