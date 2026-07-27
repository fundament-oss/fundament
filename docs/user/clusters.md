---
title: Clusters
sidebar:
  order: 5
---

A cluster is a managed Kubernetes cluster owned by your organization. It hosts
one or more [projects](./organizations.md) and the [plugins](./plugins.md)
installed on it. Clusters are managed by Gardener on top of bare-metal
infrastructure provisioned with metal-stack; see
[Infrastructure](./infrastructure.md) for the layers underneath.

## Creating a cluster

**Clusters → Add cluster** starts a three-step wizard.

### Step 1 — Cluster

| Field | Notes |
| --- | --- |
| Name | Lowercase letters, digits, `-` and `.`; must start and end with a letter or digit. Maximum 253 characters. Must be unique within the organization. |
| Region | Picked from the region catalog offered by the installation. The region determines which Kubernetes versions and machine types you can choose. |
| Kubernetes version | The versions offered by the chosen region. |

### Step 2 — Nodes

Define one or more node pools. A node pool is a group of identical machines; the
machine types on offer come from the region you picked in step 1. Use separate
pools when you need different machine sizes or want to isolate workloads onto
their own hardware.

### Step 3 — Summary

Review everything and confirm. Provisioning is asynchronous — the cluster appears
in the cluster list and moves through its states while the platform builds it.

## Regions and availability zones

Regions group the physical locations an installation offers. Within a region,
availability zones give failure isolation for workloads that need it. The
platform's position on availability zones is recorded in
[ADR 0009](/adr/0009-availability-zones).

## Node pools

Node pools can be viewed and changed from the cluster's **Nodes** tab after
creation. A pool has a name, a machine type and an autoscaler range:

| Field | Notes |
| --- | --- |
| Name | Unique within the cluster. Fixed after creation. |
| Machine type | Must be offered by the cluster's region. Fixed after creation. |
| Minimum nodes | Lower bound of the autoscaler. May be 0. |
| Maximum nodes | Upper bound of the autoscaler. Must be greater than or equal to the minimum. |

### Resizing

Editing a pool means changing its minimum and maximum. **Save changes** applies
the whole form at once: pools you added are created, pools you removed are
deleted, and pools whose bounds you changed are updated. Because name and
machine type are fixed, moving a workload to different hardware means adding a
pool with the new machine type and removing the old one — the platform does not
convert a pool in place.

### Autoscaling

The minimum and maximum are the bounds of the Kubernetes cluster autoscaler:
nodes are added when pods cannot be scheduled and removed when they are no
longer needed, within that range. The **Nodes** tab shows each pool's current
node count alongside its bounds. There is no manual "set the node count to N"
operation — set the minimum instead.

### Interaction with organization limits

The node limits on **Organization → Limits** bound what a cluster may ask for,
and they behave differently per limit:

- **Maximum nodes per node pool** silently clamps each pool's maximum (and its
  minimum, if that would end up above the clamped maximum). A pool asking for
  more nodes than the limit allows is applied at the limit rather than
  rejected.
- **Maximum node pools per cluster** and **maximum nodes per cluster** (the sum
  of all clamped pool maxima) reject the change: the cluster fails to apply
  with an "organization node limit exceeded" error instead of silently
  shrinking.

A cluster with no node pools at all still gets one default worker pool, sized
1–3 nodes.

### Applying changes

Node pool changes are asynchronous, like cluster creation: the console saves
them and the platform reconciles the cluster's worker pools with Gardener in
the background. Nodes are replaced rather than modified — adding, removing or
shrinking a pool takes the nodes involved out of service, so drain-sensitive
workloads should have a PodDisruptionBudget.

## Namespaces

Each cluster's **Namespaces** tab lists the namespaces on that cluster and which
project owns them. See [Namespaces](./namespaces.md).

## Plugins

Each cluster's **Plugins** tab shows what is installed and lets cluster admins
install more from the catalog. Plugins are per cluster: two clusters in the same
organization can run a different set, at different versions.

## Cluster access

Access to a cluster's Kubernetes API goes through a kubeconfig you download per
cluster. There is no separate kubeconfig per namespace — one kubeconfig covers
the whole cluster, and what you may actually do in it is decided per request
(see [What the kubeconfig grants](#what-the-kubeconfig-grants) below).

### Getting a kubeconfig

From the console: open the cluster and use **Download kubeconfig** in the header
of the cluster page. From the command line:

```bash
functl cluster kubeconfig <CLUSTER_ID> > ~/.kube/fundament.yaml
```

Both require view access on the cluster, which every member of the owning
organization has — organization admins and members alike. The cluster must be
ready; asking for a kubeconfig while it is still being provisioned fails with
"cluster not ready yet".

### Using it

The kubeconfig contains no long-lived credential. It points at the platform's
Kubernetes API proxy and delegates authentication to an exec credential plugin:

```yaml
users:
- name: fundament-user-<cluster-id>
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: functl
      args: [cluster, token, <cluster-id>]
```

So [`functl`](./functl.md) must be installed, on your `PATH` and logged in
(`functl auth login`) for the kubeconfig to work — including on machines that
only ever run `kubectl`. `kubectl` calls `functl cluster token` whenever it
needs a fresh token; you never handle the token yourself.

### What the kubeconfig grants

Every request goes to the API proxy rather than straight to the cluster. The
proxy checks that you may view the cluster, then swaps your platform token for
your personal ServiceAccount on that cluster — `fundament-<user-id>` in the
`fundament-system` namespace — and forwards the request. What that
ServiceAccount may do depends on your role:

| Your role | Cluster-side result |
| --- | --- |
| Organization admin | The ServiceAccount is bound to the `cluster-admin` role: full access to the cluster. |
| Member of a project on the cluster | The ServiceAccount exists but has no binding of its own, so you get only what the cluster's own RBAC grants it. |
| Neither | No ServiceAccount is created, and requests are refused. |

Two consequences worth knowing:

- The proxy forwards only the `/api`, `/apis`, `/openapi` and `/version` paths.
  Other endpoints return 404 regardless of your permissions.
- ServiceAccounts are synced in the background after a membership change, so
  immediately after being granted access a request can return 503 with
  "service account sync pending". Retry shortly.

Finer-grained in-cluster authorization — per-namespace role bindings derived
from project membership — is described as future work in FUN-7 and is not
implemented yet; see [Members and roles](./members-and-roles.md).

## Lifecycle

Cluster lifecycle management — creation, upgrades, and deletion — follows the
model described in
[ADR 0006](/adr/0006-cluster-lifecycle-management). In line with the platform's
soft-delete policy, deleting a cluster in the console removes it from your view
rather than erasing its record.
