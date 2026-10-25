---
title: Test environment with a real Gardener
sidebar:
  label: Test environment
  order: 2
---

`just dev` runs the platform in mock mode: a cluster you create exists only as a row in the database. The test environment runs the platform against a real Gardener, inside a VM, so cluster lifecycle, node pools, shoot status and cluster RBAC behave as in an installation. The whole e2e suite runs there, including the scenarios that need a real Gardener (`@real-gardener`).

```
 your machine: Linux on x86_64 with KVM, or macOS on Apple silicon
   just test-env up | sync | redeploy | test | run <sha> | ssh | down
        │ ssh, and a tunnel for https://*.fundament.localhost:8443
        ▼
 VM: NixOS, 24 GiB, up to 16 vCPU, 200 GB disk for containers
 ┌──────────────────────────────────────────────────────────────────────────┐
 │ k3d-fundament: the platform        kind gardener-operator-local: Gardener │
 │   organization-api                   virtual garden  projects, shoots     │
 │   cluster-worker  ─── kubeconfig ──► seed            shoot control planes │
 │   kube-api-proxy                     shoot nodes     run as pods too      │
 └──────────────────────────────────────────────────────────────────────────┘
```

Gardener terms used on this page:

- **Shoot**: a Kubernetes cluster that Gardener manages. Every cluster created in Fundament is a shoot.
- **Virtual garden**: Gardener's API, where projects and shoots are created.
- **Seed**: the cluster that runs each shoot's control plane as pods.

Nothing on your machine changes: Gardener's hosts entries, loopback addresses, sysctls and DNS settings are made inside the VM. The one exception is optional: `just test-env trust` adds the VM's CA to your machine's trust stores when it is not your own mkcert CA.

## Requirements

- Linux on x86_64 with virtualisation enabled in the BIOS (`/dev/kvm` exists), and Docker Engine: Docker Desktop runs containers in a VM of its own without `/dev/kvm`. Nix and qemu run in a container, so you install neither.
- Or macOS on Apple silicon with `brew install qemu xorriso`. Docker builds the VM image; once it is built, the VM boots without Docker running.
- A full clone of the repository: a shallow one cannot be pushed into the VM (`git fetch --unshallow`).
- `git`, `ssh` and `rsync`.
- A machine with 32 GB of memory: the VM takes 24 GiB (`TEST_ENV_MEM_GIB`), the smallest size in which the whole suite passes. It gets your machine's threads, up to 16 (`TEST_ENV_CPUS`).
- About 100 GB of free disk under `~/.cache/fundament/test-env`; the VM's disks grow as it uses them.
- mkcert with its CA, as `just cluster-create` sets it up. The VM signs its certificates with your CA, so your browser trusts the console. Without one, the VM makes its own; `just test-env trust` adds it to your machine.

`just test-env doctor` checks these.

## Start the environment

```shell
just test-env up
```

- It builds the VM image the first time, boots a fresh VM, copies your working tree into it, and starts the platform, Gardener and a baseline cluster `e2e-base` there. It also publishes the cert-manager plugin, which the scenarios install on `e2e-base`. It stops at the first failure and leaves the VM for you to inspect.
- It takes about 25 minutes, most of it Gardener starting. The first time takes a few minutes longer: Go builds, tools and images land on a cache disk that outlives the VM (`~/.cache/fundament/test-env/cache/`; delete it to start cold, or set `TEST_ENV_CACHE=0`).
- At the end it opens a tunnel: <https://console.fundament.localhost:8443>, logged in as `alice@acme-corp.com` with password `password`. The tunnel needs port 8443, which a local `just dev` cluster also uses; stop that first (`just cluster-stop`).
- Running `up` again needs `just test-env down` first. Every `up` is a clean start.

## Run the e2e scenarios

```shell
just test-env test
```

```shell
just test-env test @cluster-lifecycle
```

- It runs the whole e2e suite (the `full` profile in `e2e/cucumber.json`) against the real Gardener, or the scenarios a tag expression selects. PR previews run the `default` profile, which leaves the `@real-gardener` scenarios out.
- A scenario tagged `@known-bug` fails on a filed bug, linked in its description. While it fails it shows as skipped, with the issue and the error in the report; once it passes it fails, so the tag gets removed.
- The report, a Playwright trace per failed scenario and the logs of fundament, Gardener and the shoots land in `test-env/bundles/<time>-<sha>/`. Open a trace with `bunx playwright show-trace <zip>`.

To run one commit on a fresh VM, without your uncommitted changes:

```shell
just test-env run HEAD
```

It removes the VM when the run is green and keeps it when it is red. A whole run takes about 45 minutes.

## Change code during a run

```shell
just test-env redeploy
```

- It copies your working tree into the VM and redeploys fundament without resetting the databases, so clusters and their shoots stay.
- `just test-env sync` only copies.

## Inside the VM

```shell
just test-env ssh
```

The shell starts in the VM's checkout, with the repository's tools. Its kube contexts:

| Context | Cluster |
|---|---|
| `k3d-fundament` | the platform |
| `kind-gardener-local-garden` | virtual garden: projects and shoots |
| `kind-gardener-operator-local` | the kind cluster, with one `shoot--<project>--<name>` namespace per shoot |
| `shoot-e2e-base` | the baseline cluster |

`just test-env shoots`, `just test-env logs`, `just test-env gardener-status` and `just test-env shoot-context <cluster-id>` work in the VM only, as do the other `gardener-*` recipes.

The environment deletes clusters immediately (`clusterWorker.gardenerImmediateClusterDeletion`): Gardener does not wait for what runs in a cluster to clean up. To see how a deletion behaves without it, turn it off in the VM; the next `redeploy` turns it on again:

```shell
kubectl --context k3d-fundament -n fundament set env deploy/cluster-worker GARDENER_IMMEDIATE_CLUSTER_DELETION=false
```

## Plugins on a cluster

Every cluster gets the plugin-controller, as in an installation. `up` publishes cert-manager. To install another first-party plugin, publish and approve it in the VM:

```shell
just test-env ssh just test-env plugin-publish <plugin>
```

- `<plugin>` is its path under `plugins/`. It builds the plugin's image, publishes its `definition.yaml` as `platform-admin@fundament.io` and approves the version. It prints the version and hash.
- Then install it as `alice@acme-corp.com`: from the console's **Plugins** page, or with the Terraform resource `fundament_plugin_installation`.
- Versions are create-only: bump `metadata.version` in `definition.yaml` to publish a change.

## Stop

```shell
just test-env down
```

This removes the VM and its disks. A stopped VM is not started again: its container disk is wiped on every boot.

## Remove a Gardener from your machine

A local Gardener that runs directly on your machine (the kind cluster `gardener-operator-local`) is removed with:

```shell
just test-env gardener-delete
```

This deletes the kind cluster, the registry and DNS containers Gardener runs next to it, the Gardener clone and the two kubeconfig contexts. The `/etc/hosts` entries, the DNS settings and the addresses on `lo` stay.

## Other providers

`TEST_ENV_PROVIDER` picks where the VM runs. `qemu` (the default on Linux) and `macos` (the default on macOS) run it on your machine. `hetzner` creates a Hetzner Cloud server and installs the same NixOS on it, with the token in `test-env/host/providers/hetzner.env`; it has not been tried yet. `test-env/README.md` describes how a provider plugs in.
