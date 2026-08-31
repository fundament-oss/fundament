# Plugin Sandbox

`sandbox/` holds a development environment for plugin work: the `k3d-fundament-plugin` sandbox
cluster, running only the plugin-controller, with its own registry on port `5112` so it coexists
with the `k3d-fundament` platform cluster.

The sandbox is not self-contained. A plugin is installed by publishing its **definition** to
organization-api and referencing it from a `PluginInstallation` (FUN-19), so the management
cluster — database, organization-api, OpenFGA and a dex login — has to be running too. See
[Install a plugin](../docs/developer/plugins/install-a-plugin.md) for the walkthrough, and
[The two development clusters](../docs/developer/plugins/dev-environment.md) for how the two
clusters fit together. This file is the command reference.

## Quick start

The recipes here live in `mod.just`, which the repository root's `Justfile` registers as the
`plugins` module. So they are **run from the repository root**, prefixed with `plugins` — a bare
`just cluster-create` from any directory resolves to the root recipe and builds the *platform*
cluster instead.

```shell
just plugins cluster-create   # K3D cluster + registry (~10s)
just plugins dev              # build + deploy plugin-controller, with file watching
just plugins deploy           # one-time build without file watching
just plugins sandbox-orgapi   # bridge the controller to organization-api
```

Then publish a definition and install it. `publish` builds the image, pushes it and
registers the definition, printing the `definitionHash` the `PluginInstallation` must pin:

```shell
export PLUGIN_REGISTRY=localhost:5112
export FUNDAMENT_ORG_API_URL=https://organization.fundament.localhost:8443
export FUNDAMENT_ORGANIZATION_ID=019b4000-0000-7000-8000-000000000000   # seeded "system" org
export FUNDAMENT_TOKEN=$(./deploy/k3d/dev-token.sh)

just plugins publish cert-manager
# published plugin=cert-manager version=… hash=sha256:… id=… definition_id=…
```

```shell
kubectl --context k3d-fundament-plugin apply -f - <<'YAML'
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: system--cert-manager     # must be <organizationName>--<pluginName>
spec:
  definitionRef:
    organizationName: system
    pluginName: cert-manager
    pluginVersion: …             # printed by publish
    definitionHash: sha256:PASTE_THE_HASH
YAML
```

Republishing an existing `pluginVersion` needs `--replace`, which soft-deletes the previous
definition. The printed hash covers the manifest *including the freshly built image's digest*, so
it changes whenever the rebuild is not byte-identical — after a cold rebuild, update
`definitionHash` in any `PluginInstallation` that pins it.

Always set `definitionHash`. `pluginController.allowUnpinnedHash: true` in the sandbox implies it
is optional, but an absent hash trips the CRD's `definitionRef` immutability rule on the
controller's first update and the installation silently never reconciles.

Each plugin has its own `pluginVersion`, and the `publish` argument is a **path** under
`plugins/` rather than the plugin name. The table of both, per plugin, is in
[Install a plugin](../docs/developer/plugins/install-a-plugin.md#other-plugins).

## Working with an installed plugin

`logs` and `uninstall` take the installation's `metadata.name` — the `<organization>--<plugin>`
pair, not the plugin name alone.

```shell
just plugins status                          # all PluginInstallation CRs
just plugins logs system--cert-manager       # stream one plugin's logs
just plugins logs                            # plugin-controller logs
just plugins uninstall system--cert-manager  # delete the PluginInstallation
just plugins cluster-delete
```

Per-plugin verification lives in each plugin's own module:

```shell
just plugins cert-manager test             # self-signed ClusterIssuer + Certificate
just plugins cert-manager test-cleanup
just plugins openfsc operator-push         # build the openfsc-operator image for the sandbox
just plugins openfsc test                  # sample FSCInstallation reaches Active
just plugins openfsc test-cleanup
```

For `ceph-rook`, which needs raw block devices and a longer setup, follow the
[verification runbook](../docs/developer/plugins/ceph-rook-runbook.md).

## external-dns is not installable

`plugins/external-dns/` has source, a `Justfile` and test resources, but it is **not** registered
as a `mod` in this directory's `mod.just` and has **no** entry in
`db/seed/0101-appstore-catalog.sql`. So `just plugins external-dns test` does not resolve, and
`just plugins publish external-dns` fails with `no catalog entry for "external-dns"`.


All commands are defined in `mod.just`; run `just --list plugins` to see them.
