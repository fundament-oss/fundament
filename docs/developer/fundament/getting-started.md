---
title: Getting started with Fundament development
sidebar:
  label: Getting started
  order: 1
---

## Prerequisites

- [Mise](https://mise.jdx.dev): installs the pinned `just`, `k3d`, `kubectl`, `helm`, `skaffold`, `mkcert`, `go` and `bun` from `mise.toml`
- [Docker](https://www.docker.com) with at least 16 GiB of memory; 24 GiB recommended when you also run the plugin sandbox
- `certutil` (NSS tools), which `mkcert` needs to install the local CA:

| OS | Install |
|---|---|
| macOS | `brew install nss` |
| Debian/Ubuntu | `apt install libnss3-tools` |
| Fedora/RHEL | `dnf install nss-tools` |
| Arch | `pacman -S nss` |

### macOS

The default shared memory limits are too low for PostgreSQL. For embedded-postgres, create/edit `/etc/sysctl.conf`:

```
kern.sysv.shmall=65536
kern.sysv.shmmax=16777216
```

### Windows

Use PowerShell 7 (`winget install Microsoft.PowerShell`); `mise activate` depends on a hook it does not have on Windows PowerShell 5.1, where it silently fails to populate `PATH`.
Check your powershell version with `$PSVersionTable.PSVersion`.
If the profile refuses to load, run `Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned`.

Add to `$PROFILE`:

```powershell
$env:PATH = "C:\Program Files\Git\bin;C:\Program Files\Git\usr\bin;$env:PATH"
mise activate pwsh | Out-String | Invoke-Expression
```

Those `PATH` entries supply the `bash` and `cygpath` that `just` recipes need; the `postinstall` hook `just e2e::install` fails until they are present, so rerun it afterwards.

Use Edge or Chrome for the console: `mkcert` cannot install its CA into Firefox on Windows (no NSS `certutil`), so calls to `authn.fundament.localhost` fail.

If `just dev` leaves pods in `ImagePullBackOff`, run `just dev --default-repo=localhost:5111`; Skaffold's k3d registry auto-detection can override the `SKAFFOLD_DEFAULT_REPO` the recipe sets.

## Install the tools

```shell
mise trust
mise install
```

Run `mise install` without arguments; naming a tool, as in `mise install go`, rewrites that tool's pinned version in `mise.toml` to `latest`.

## Run the platform

```shell
just cluster-start
just dev-hotreload
```

- `cluster-start` creates the `k3d-fundament` cluster on first run, installs the local CA and switches the kubectl context to it.
- `dev-hotreload` builds and deploys every service, then stays attached.
  - Edits to existing files are synced into the running containers.
  - Adding, moving or deleting a file rebuilds the image and redeploys.
- Every deploy resets the databases to the test data.
- `Ctrl-C` stops watching; the deployment keeps running.

To check the production images (the Dockerfiles CI builds), run `just dev` once and stop it with `Ctrl-C`: it rebuilds and redeploys on every file change.

## Open the console

<https://console.fundament.localhost:8443>

| User | Password | Organization |
|---|---|---|
| `alice@acme-corp.com` | `password` | `acme-corp`, with cluster `acme-cluster` |
| `platform-admin@fundament.io` | `password` | `system`, which owns the first-party plugins |

See [`console-frontend/README.md`](https://github.com/fundament-oss/fundament/blob/master/console-frontend/README.md) for the frontend commands.

## Edit the docs

Under `just dev-hotreload`, <https://docs.fundament.localhost:8443> shows edits to existing pages in `docs/` within seconds. Without the cluster:

```shell
just docs-dev     # dev server on http://localhost:4321
just docs-build   # production build, fails on broken links
```

## Remove

```shell
just cluster-delete
```

## Next

- [Plugins](/docs/developer/plugins): write a plugin, run it locally
- [Block devices for k3d](./k3d-block-devices.md): storage plugin work
