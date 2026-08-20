---
title: Getting started with Fundament development
sidebar:
  label: Getting started
  order: 1
---

## Prerequisites

- [Mise](https://mise.jdx.dev)
- [Just](https://just.systems)
- [Docker](https://www.docker.com)
- `certutil` (part of NSS tools). Required for `mkcert` to install the CA into system trust stores

Every other tool these docs use is provided by mise — see `mise.toml` for the list. The
commands throughout assume an activated mise environment; without one they are not on `PATH`.
Either [activate mise in your shell](https://mise.jdx.dev/getting-started.html) or prefix
commands with `mise x --`.

### Installing certutil

**macOS:**

```shell
brew install nss
```

**Debian/Ubuntu:**

```shell
apt install libnss3-tools
```

**Fedora/RHEL:**

```shell
dnf install nss-tools
```

**Arch:**

```shell
pacman -S nss
```

## MacOS

On macOS, the default shared memory limits are too low for PostgreSQL.
For embedded-postgres, create/edit `/etc/sysctl.conf`:

```
kern.sysv.shmall=65536
kern.sysv.shmmax=16777216
```

## Installation

```shell
mise trust
mise install
```

## Run cluster

From the repository root:

```shell
just cluster-start
just dev
```

`cluster-start` creates the **platform** cluster `k3d-fundament` and installs the mkcert CA as a
cert-manager issuer.
On a machine with no mkcert CA yet, `mkcert -install` asks for sudo to add it to the system
trust store.

`just dev` is `skaffold dev`. It builds the platform images, pushes them to the local registry,
deploys the chart, and then **stays attached to your terminal watching your source files** —
edit a Go file and it rebuilds that one image and redeploys it, streaming the pods' logs
meanwhile. On a cold Docker cache expect about ten minutes: roughly a minute for the cluster,
six to build the images, and a few more for them to roll out. Nothing is pulled from a registry;
every image is built locally.

Once the deployments are stable you can **stop it with Ctrl-C** — the recipe passes
`--cleanup=false`, so nothing is torn down. The cluster keeps running and the console keeps
serving. Leave it running only if you are editing Fundament's own source and want your changes
rebuilt automatically; if you are here to work on plugins, you do not need it.

:::caution[Stopping is safe — restarting is not]
Running `just dev` again re-runs the database migrations, and `values-local.yaml` sets
`db.reset: true`, so it **wipes and re-seeds the platform database**. Every published plugin
definition goes with it, and the installed plugin keeps reporting `Running` until its controller
next restarts. Once you have published a plugin, avoid re-running `just dev` unless you mean to
start over — see
[Install a plugin](../plugins/install-a-plugin.md#known-issues-on-this-path).
:::

## What you just started

`k3d-fundament` is the **platform** cluster. It runs the Fundament services themselves: a
CloudNativePG Postgres, `authn-api`, `organization-api`, `authz-worker` with OpenFGA, `dex`, the
console and docs frontends, `kube-api-proxy`, `plugin-proxy`, `plugin-controller` and
ingress-nginx.

Plugins do **not** run here. They run on a second, separate k3d cluster that you create later —
in production that is a managed tenant cluster, and locally it is `k3d-fundament-plugin`. See
[The two development clusters](../plugins/dev-environment.md).

## Where things are

Once `just dev` reports the deployments stable:

| | URL | Sign in with |
|---|---|---|
| Console | <https://console.fundament.localhost:8443> | `alice@acme-corp.com` / `password` |
| Documentation | <https://docs.fundament.localhost:8443> | — |

Other services follow the same pattern — `authn`, `organization`, `dex` and `plugin-proxy` are
each at `https://<name>.fundament.localhost:8443`. Publishing plugins uses a different identity,
`platform-admin@fundament.io`, minted by `deploy/k3d/dev-token.sh`.

These docs are served from the cluster you just started, so the documentation URL above is your
branch's content. The published site — built from `master` — is at
<https://docs.fundament.projects.digilab.network/docs>. To work on the docs themselves, `just docs-dev` runs a live
dev server on <http://localhost:4321>, and `just docs-build` validates every internal link and
fails on a broken one. The pages live in `docs/`; the link-writing rules are in
[`docs-frontend/README.md`](https://github.com/fundament-oss/fundament/blob/master/docs-frontend/README.md).

## Next: install a plugin

Working on the console frontend itself? See
[`console-frontend/README.md`](https://github.com/fundament-oss/fundament/blob/master/console-frontend/README.md)
for linting, formatting and the other frontend commands.

Otherwise the next step is [Install a plugin](../plugins/install-a-plugin.md), which takes you
from the running platform to `cert-manager` installed and visible in the console. Read
[The two development clusters](../plugins/dev-environment.md) first if you have not already.

## Storage

Only if you are working on the `ceph-rook` plugin: it needs raw block devices, which a k3d node
does not have. See [Block devices for k3d](./k3d-block-devices.md). This is not part of the
plugin walkthrough.
