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

## Windows

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

## Installation

```shell
mise trust
mise install
```

Run `mise install` without arguments; naming a tool, as in `mise install go`, rewrites that tool's pinned version in `mise.toml` to `latest`.

## Run cluster

```shell
just cluster-start
just dev
```

## Console frontend

The Console frontend should now be available at https://console.fundament.localhost:8443/. See
[`console-frontend/README.md`](https://github.com/fundament-oss/fundament/blob/master/console-frontend/README.md)
for linting, formatting and the other frontend commands.

## Storage

Working on the `ceph-rook` plugin needs raw block devices, which a k3d node does not have.
See [Block devices for k3d](./k3d-block-devices.md).
