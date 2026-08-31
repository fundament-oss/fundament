---
title: The two development clusters
sidebar:
  label: Dev environment
  order: 2
---

Plugin development uses **two k3d clusters**: the `k3d-fundament` platform cluster and the
`k3d-fundament-plugin` sandbox cluster. Knowing which is which, and which `just` recipe targets
which, is most of what makes the rest of these pages make sense.

Throughout these docs a cluster is named by its kubectl context — `k3d-fundament` or
`k3d-fundament-plugin` — with "platform" or "sandbox" as a descriptor. The context name is the
unambiguous part, and it is what every `kubectl --context` command already uses.

```
  ┌─ k3d-fundament ─────────────────┐        ┌─ k3d-fundament-plugin ─────────┐
  │ the platform                    │        │ the sandbox                    │
  │ just <recipe>                   │        │ just plugins <recipe>          │
  ├─────────────────────────────────┤        ├────────────────────────────────┤
  │ fundament namespace             │        │ fundament namespace            │
  │   organization-api              ◄──(1)───┤   plugin-controller            │
  │   authn-api · dex               │        │        │ creates               │
  │   authz-worker + OpenFGA        │        │        ▼                       │
  │   CloudNativePG (db)            │        │ plugin-<org>--<name> namespace │
  │   console-frontend              │        │   the plugin's Deployment      │
  │   docs-frontend                 │        │   + what the plugin installs   │
  │   plugin-proxy · kube-api-proxy ├──(2)───►     (cert-manager, Rook, …)    │
  │ ingress-nginx                   │        │                                │
  │   *.fundament.localhost:8443    │        │ no ingress                     │
  │ registry  localhost:5111        │        │ registry  localhost:5112       │
  └─────────────────────────────────┘        └────────────────────────────────┘
```

The clusters sit on separate Docker networks, so the two links between them are created by
hand. **Both must be re-run after recreating either cluster** — each resolves a container IP
that changes.

**(1) `just plugins sandbox-orgapi`** — the plugin-controller resolves a
`PluginInstallation` by fetching its published definition from organization-api (FUN-19).

**(2) `just plugin-sandbox-kubeconfig`** — a root recipe, then restart
plugin-proxy and kube-api-proxy. One Secret, read by both: it is what lets the console see and
manage what is installed in the sandbox (FUN-17). It does more than it sounds like — locally
kube-api-proxy runs in `mock` mode serving an in-memory fake cluster, and this Secret replaces
that mock with a real proxy to the sandbox. Until it exists the console shows fabricated data
and plugin pages fail to load. It is mounted optionally, so plugin-proxy starts and reports
healthy without it.

This mirrors production: Fundament manages *other* clusters, and a plugin runs on the managed
cluster, not on the platform itself.

## Which `just dev` am I running?

Every recipe is run from the **repository root**. The sandbox ones live in `plugins/mod.just`,
which the root `Justfile` registers as the `plugins` module, so they carry a `plugins` prefix.
Both files define a `dev`, and they deploy different things to different clusters:

| Recipe | Deploys | To |
|---|---|---|
| `just dev` | the full platform | `k3d-fundament` |
| `just plugins dev` | plugin-controller only | `k3d-fundament-plugin` |

The prefix is the only thing that distinguishes them, and `cd`-ing into `plugins/` does **not**
select the sandbox one: `mod.just` is not a `Justfile`, so `just` keeps walking up and finds the
root recipe. A bare `just cluster-create` from anywhere in the tree builds the *platform*
cluster.

Nor does your kubectl context matter — both skaffold configurations pin `kubeContext`, so
`kubectl config use-context` has no effect on where they deploy.

`just --list plugins` shows what the module offers.

## How long it takes

On a cold Docker cache, from no images at all:

| Step | Time |
|---|---|
| `just cluster-start` (platform) | ~1 min |
| `just dev` (platform, 14 images) | ~6 min to build, a few more to roll out |
| sandbox cluster + `just plugins dev` | ~1 min |
| `just plugins publish` (first plugin) | ~4 min |

Measured on an 8-CPU macOS VM after `docker system prune -a`, so nothing was cached. Around
fifteen minutes in total before a plugin is installable; later runs reuse the build cache and are
much quicker. There is no prebuilt-image path — every platform image is built locally.

## Next

- Prerequisites and the platform cluster: [Getting started](../fundament/getting-started.md)
- The commands, in order: [Install a plugin](./install-a-plugin.md)
- Raw block devices, for the `ceph-rook` plugin only:
  [Block devices for k3d](../fundament/k3d-block-devices.md)
