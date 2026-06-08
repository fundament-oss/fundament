# OpenFSC plugin

Installs OpenFSC with a Manager, Controller, Inway and Outway on the Cluster.

## Configuration

The plugin installs with development configuration by default.
Available settings:

| Key | Default | Description |
| --- | --- | --- |
| `GROUP_ID` | `fsc-demo` | FSC group of the directory peer |
| `DIRECTORY_PEER_ID` | `12345678901234567899` | directory peer ID (CA cert serial number) |
| `FSC_NAMESPACE` | `fsc` | namespace of the Manager/Controller |
| `MANAGER_ADDRESS` | `https://shared-open-fsc-manager-external.fsc:8443` | in-cluster Manager address |
| `CONTROLLER_URL` | `http://localhost:9080` | host-reachable Controller UI (empty hides the link) |
| `CONTROLLER_ADMIN_ADDRESS` | _(auto)_ | Controller Administration API (mTLS) |
| `CONTROLLER_SERVER_NAME` | `shared-open-fsc-controller` | name verified against the Controller TLS cert |
| `FSC_CERT_SECRET` | _(auto)_ | `namespace/name` of the mTLS client bundle for the Admin API |
| `FSC_INSECURE` | `false` | skip server-cert verification (dev only) |

These config settings should be updated in the `../Justfile`, where `kubectl apply` is called.
Update the heredoc and add a `config:` block under `spec:`.

Example:

```yaml
    spec:
      image: ${image}
      ...
      config:
        GROUP_ID: "my-group"
        FSC_INSECURE: "true"
```

Adjust these settings before you run `just plugin-install openfsc`.

## Install

From `plugins/`:

```sh
just cluster-create          
just dev                     # run the plugin-controller (leave running)
just plugin-install openfsc  # build and apply the PluginInstallation
```

## Test

Tests the FSC Peer named `self` is running (registered).

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
