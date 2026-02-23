---
title: Development setup
---

## Prerequisites

- [Mise](https://mise.jdx.dev)
- [Just](https://just.systems)
- [Docker](https://www.docker.com)

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
