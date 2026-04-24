---
title: Development setup
sidebar:
  order: 10
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

## Installation

```shell
mise trust
mise install
```

## Run cluster

```shell
just cluster-start
just dev
```

## Console Frontend

See `console-frontend/README.md`.
