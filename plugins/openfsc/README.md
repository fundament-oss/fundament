# OpenFSC plugin

Installs OpenFSC with a Manager, Controller, Inway and Outway on the cluster.

The plugin is a thin installer around the standalone
[`openfsc-operator`](../../openfsc-operator/README.md): it installs the
prerequisite operators (cert-manager, CloudNativePG), the `basic-csi`
StorageClass and the vendored openfsc-operator chart, then declares the
directory as a cluster-scoped `Directory` resource and seeds a
`default-inway`/`default-outway` pair. The operator deploys the OpenFSC core
(Manager, Controller, auditlog, txlog-api, group CA, Postgres) from the
`Directory` and provisions one gateway per `Inway`/`Outway` resource — and it
keeps reconciling them when this plugin pod is down.

## Configuration

The plugin installs with development configuration by default.
Available settings:

| Key | Default | Description |
| --- | --- | --- |
| `GROUP_ID` | `fsc-demo` | FSC group of the directory peer |
| `DIRECTORY_PEER_ID` | `12345678901234567899` | directory peer ID (group cert serial number) |
| `FSC_NAMESPACE` | `fsc` | namespace of the Manager/Controller |
| `CONTROLLER_URL` | `http://localhost:9080` | host-reachable Controller UI (empty hides the link) |
| `OPERATOR_IMAGE` | `ghcr.io/fundament-oss/fundament/openfsc-operator:latest` | openfsc-operator image the vendored chart deploys |

In the sandbox, write `KEY=VALUE` lines to `.sandbox-config` (next to this
README); `just plugin-install openfsc` turns them into
`PluginInstallation.spec.config`. The `operator-push` recipe records
`OPERATOR_IMAGE` there automatically.

## Install

From `plugins/`:

```sh
just cluster-create
just dev                       # run the plugin-controller (leave running)
just openfsc::operator-push    # build + push the operator image, record OPERATOR_IMAGE
just plugin-install openfsc    # build and apply the PluginInstallation
```

## Test

Waits for the `Directory`, the `self` Peer and the default gateways to be
Active.

```sh
just openfsc test
```

## Console preview

```sh
just openfsc console-preview # http://localhost:4319
```

Serves `console/` against the live cluster via a kubectl-backed SDK stand-in (no Console host).

## Cleanup

```sh
just openfsc test-cleanup
just plugin-uninstall openfsc
```
