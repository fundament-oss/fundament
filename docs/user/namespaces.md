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

A project can own several namespaces, for example one per environment, but
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

- **Short enough for the project prefix.** The name on the cluster is
  `tnt-<project>--<your-name>` and must fit the Kubernetes limit of 63
  characters, so your name can be at most `57 - length of the project name`
  characters. Project names are at most 30 characters, so that is always at
  least 27.
- **Unique within the project.** Two projects may each have a `staging`.
- **Not a system name.** `default`, `kube-system`, `kube-public`,
  `kube-node-lease` and `fundament-system` are rejected, as is anything
  starting with `kube-`.

### The name on the cluster

The name you choose is the name you see everywhere in the console, the API and
`functl`. On the cluster itself the namespace is created with `tnt-` and the
project's name in front:

```
tnt-<project>--<your-name>
```

So a `staging` namespace in a project named `payments` becomes
`tnt-payments--staging` on the cluster. The project name keeps two projects'
`staging` namespaces apart; `tnt-` (tenant) keeps your namespaces apart from
the cluster's own, such as `kube-system` or the namespaces plugins install
into, and groups them together in `kubectl get ns`. The name never changes,
because project and namespace names can't be changed after creation.

This matters when you use `kubectl`: the console shows `staging`, but
`kubectl get ns` shows `tnt-payments--staging`, and that is the name you pass
to `kubectl -n`.

Namespaces created before this naming was introduced keep their earlier
cluster name, a project prefix with a few generated characters (for example
`payments1f3a-staging`); Kubernetes can't rename a namespace.

## Quotas and limits

Resource limits are set on **Organization → Limits** and **Project → Limits**.
See [Members and roles](./members-and-roles.md) for who is allowed to change
them. There are two kinds, and only the second one reaches namespaces.

The values in the tables below are the platform's starting values, offered in
the console and restored by **Reset to defaults**. They are not floors: a limit
left unset means no limit at all, not the value listed here.

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
both levels set the same field, the **lower value wins**: a project can only
tighten what the organization allows, never loosen it. A field left unset at
both levels means no default is applied for it, and if no field is set at all
the LimitRange is not created.

### What happens when you hit them

Nothing is rejected. These are *defaults*, not caps: a container that specifies
no CPU or memory request and limit of its own gets these values, and a container
that specifies its own keeps them, however large. Storage and object counts are
not limited at all.

Where a namespace does run out of room is at the cluster level: pods stay
`Pending` when the cluster cannot grow enough nodes to schedule them, which is
where the node limits above come back in.
