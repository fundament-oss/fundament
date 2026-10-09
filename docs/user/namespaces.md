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
- `functl namespace` and the [OpenTofu provider](./opentofu-provider.md)
  manage the same namespaces: `fundament_namespace` creates and deletes one,
  `fundament_project_namespaces` and `fundament_cluster_namespaces` list them.

A namespace created directly in the cluster, for example with
`kubectl create namespace`, is not a Fundament namespace: it belongs to no
project, Fundament does not list it, and project members get no rights in it.

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

## Defaults

What a container gets when its workload sets no resources of its own is set in
two places:

- on a cluster's own page, which applies to every namespace on that cluster
- on a project's **General** page, which applies to that project's namespaces
  only

See [Members and roles](./members-and-roles.md) for who may change them.
Organization admins set a cluster's defaults; a project admin sets the
project's.

### The four values

| Default | Unit | Suggested |
| --- | --- | --- |
| CPU request | millicores | 100 |
| CPU limit | millicores | 500 |
| Memory request | mebibytes | 256 |
| Memory limit | mebibytes | 512 |

The suggested column is what the console offers on a cluster as a starting
point. They are not floors: a field left unset applies no default at all, not
the value listed here.

### Inheritance, and the one rule

A field the project does not set takes the cluster's value, so a project only
has to say what it wants to differ. A field neither sets applies no default.

A project's value may be **lower than the cluster's, never higher**. That holds
per field, and only where the cluster has a value at all: where the cluster sets
no CPU limit, a project may set any CPU limit it likes.

Two things are refused when you save them, with the offending value named:

- A project value above the cluster's. Lower the project, or raise the cluster
  first.
- Lowering a cluster value below a value one of its projects has set. The error
  names the project, so you know which one to lower first. Nothing is cascaded:
  a project's value is never changed on its behalf.

The effective request may not exceed the effective limit either, even when each
of the two comes from a different place. A cluster with a CPU limit of 200m and
a project with a CPU request of 300m and no limit of its own is refused for that
reason.

### What happens when you hit them

Nothing is rejected. These are *defaults*, not caps: a container that specifies
no CPU or memory request and limit of its own gets these values, and a container
that specifies its own keeps them, however large. Storage and object counts are
not limited at all.

The effective values land in every namespace as a Kubernetes LimitRange named
`fundament-defaults`. When no field applies, the LimitRange is not created, and
clearing every field removes it again.

Where a namespace does run out of room is at the cluster level: pods stay
`Pending` when the cluster cannot grow enough nodes to schedule them. See
[Clusters](./clusters.md) for how a cluster grows.
