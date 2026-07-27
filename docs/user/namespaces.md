---
title: Namespaces
sidebar:
  order: 6
---

Namespaces are where your workloads actually run. A [project](./organizations.md)
owns one or more namespaces on the cluster it runs on, and namespaces are the
unit that Kubernetes RBAC and resource quotas apply to.

## How projects map onto namespaces

```
Organization
└── Cluster
    └── Project (runs on exactly one cluster)
        └── Namespace(s) on that cluster
```

A project can own several namespaces — for example one per environment — but
every one of them lives on the project's cluster. A cluster hosts the namespaces
of all the projects assigned to it.

Because [plugins](./plugins.md) are installed per cluster, every namespace on a
cluster has access to the same set of plugins.

## Managing namespaces

- **Project → Namespaces** lists the namespaces owned by a project and lets you
  add new ones.
- **Cluster → Namespaces** shows every namespace on a cluster together with the
  project that owns it, which is the view cluster admins use.

## Naming

Namespace names follow the Kubernetes rules: lowercase letters, digits and `-`,
starting and ending with a letter or digit. On top of that the platform
requires:

- **At most 50 characters** — shorter than the Kubernetes limit of 63, because
  of the prefix described below.
- **Unique within the project.** Two projects may each have a `staging`.
- **Not a system name.** `default`, `kube-system`, `kube-public`,
  `kube-node-lease` and `fundament-system` are rejected, as is anything
  starting with `kube-`.

### The name on the cluster

The name you choose is the name you see everywhere in the console, the API and
`functl`. On the cluster itself the namespace is created with a project prefix:

```
<project-prefix>-<your-name>
```

where the prefix is 12 characters: up to 8 from the project's name, followed by
4 characters derived from the project's ID. So a `staging` namespace in a
project named `payments` becomes something like `payments1f3a-staging` on the
cluster.

The prefix is what lets several projects share one cluster: it keeps two
projects' `staging` namespaces apart even when their names are similar, and the
ID-derived part keeps them apart when the names are identical. It is
deterministic and never changes, because it is derived only from values that
cannot change after creation.

This matters when you use `kubectl`: the console shows `staging`, but
`kubectl get ns` shows `payments1f3a-staging`, and that is the name you pass to
`kubectl -n`.

## Quotas and limits

Resource limits are set on **Organization → Limits** and **Project → Limits**.
See [Members and roles](./members-and-roles.md) for who is allowed to change
them. There are two kinds, and only the second one reaches namespaces.

### Node limits (organization only)

| Limit | Default | Effect |
| --- | --- | --- |
| Maximum nodes per cluster | 10 | Sum of all node pool maxima in a cluster |
| Maximum node pools per cluster | 5 | Number of node pools in a cluster |
| Maximum nodes per node pool | 5 | Upper bound of a single pool's autoscaler |

These bound the hardware a cluster may grow to, not what a namespace may
consume. See [Clusters](./clusters.md#interaction-with-organization-limits) for
how each one is enforced.

### Per-container resource defaults

| Limit | Default | Unit |
| --- | --- | --- |
| Default CPU request | 100 | millicores |
| Default CPU limit | 500 | millicores |
| Default memory request | 256 | mebibytes |
| Default memory limit | 512 | mebibytes |

These are set at both the organization and the project level, and they land in
every namespace as a Kubernetes LimitRange named `fundament-defaults`. Where
both levels set the same field, the **lower value wins** — a project can only
tighten what the organization allows, never loosen it. A field left unset at
both levels means no default is applied for it, and if no field is set at all
the LimitRange is not created.

### What happens when you hit them

Nothing is rejected. These are *defaults*, not caps: a container that specifies
no CPU or memory request and limit of its own gets these values, and a container
that specifies its own keeps them, however large. Storage and object counts are
not limited at all.

Where a namespace does run out of room is at the cluster level — pods stay
`Pending` when the cluster cannot grow enough nodes to schedule them, which is
where the node limits above come back in.
