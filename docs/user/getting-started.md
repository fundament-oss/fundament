---
title: Getting started
sidebar:
  order: 3
---

This page walks through the first things you do as a new Fundament user: signing
in, finding your organization, launching a cluster, creating a project and
installing a plugin.

If you are looking to build plugins on top of Fundament rather than use it, see the
[Developer documentation](../developer/development-setup.md) instead.

## Before you start

You need:

- An account in a Fundament installation, and the URL of its console.
- Membership of at least one organization. Organizations are not self-service:
  an existing organization admin invites you. See
  [Members and roles](./members-and-roles.md).

## 1. Sign in

Open the console and sign in. Authentication is handled by the platform's
identity provider, so you use the same credentials as for the rest of your
organization's tooling.

After signing in you land on the dashboard. If you belong to more than one
organization, use the organization picker in the header to switch between them —
everything else in the console is scoped to the organization you have selected.

## 2. Create a cluster

Every project runs on a cluster, so the cluster comes first. If your organization
already has one you can use, skip to step 3.

Go to **Clusters → Add cluster**. The wizard has three steps:

1. **Cluster** — name, region and Kubernetes version.
2. **Nodes** — one or more node pools, each with a machine type and a size.
3. **Summary** — review and confirm.

Provisioning takes a while; the cluster shows up in the list with its state
while it is being created. See [Clusters](./clusters.md) for what the individual
fields mean and how to change them later.

## 3. Create a project

A project is where your workloads live. Each project runs on exactly one cluster,
and a cluster can host multiple projects; see
[Organizations and projects](./organizations.md) for the full resource model.

Go to **Projects → Add project**, give the project a name and pick the cluster it
should run on.

## 4. Install a plugin

Capabilities such as databases, ingress and object storage are delivered as
Plugins, installed per cluster. Open the cluster and go to **Plugins**, then pick
one from the catalog and install it.

See [Plugins](./plugins.md) for the catalog, the support tiers and how
versioning works.

## 5. Deploy a workload

Your project maps onto one or more namespaces on its cluster. Add one under
**Project → Namespaces**, then deploy into it with the usual Kubernetes tooling.

Credentials are per cluster, not per namespace. Install the
[functl CLI](./functl.md) and log in, then fetch a kubeconfig:

```bash
functl auth login
functl cluster kubeconfig <CLUSTER_ID> > ~/.kube/fundament.yaml
export KUBECONFIG=~/.kube/fundament.yaml
kubectl get ns
```

The console offers the same file behind **Download kubeconfig** on the cluster
page. Either way `functl` has to stay installed and logged in: the kubeconfig
holds no credential of its own and calls `functl` for a fresh token on every
request.

Two things that surprise people the first time:

- The namespace is called something else on the cluster. A `staging` namespace
  in project `payments` shows up in `kubectl` as `payments1f3a-staging` — see
  [Namespaces](./namespaces.md#the-name-on-the-cluster).
- What you may do depends on your role. Organization admins get full access;
  project members get only what the cluster's RBAC grants them. See
  [Cluster access](./clusters.md#cluster-access).

## Next steps

- Automate the above from the command line with the [functl CLI](./functl.md),
  or declaratively with the [OpenTofu provider](./opentofu-provider.md).
- Create an [API key](./api-keys.md) for scripts and CI.
- Read the [Overview](./overview.md) for the thinking behind the platform.
