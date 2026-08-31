---
title: Install a plugin
sidebar:
  order: 3
---

Install `cert-manager` into the local `k3d-fundament-plugin` sandbox cluster, prove it works,
and see it in the console. This is the standard path for working with plugins locally, and the
one to follow first.

About fifteen minutes from a cold Docker cache, much less with a warm one — see the
[timings](./dev-environment.md#how-long-it-takes).

**Before you start:** the platform must be running —
[Getting started](../fundament/getting-started.md) — and it is worth reading
[The two development clusters](./dev-environment.md) first, because every step below touches one
of the two clusters and they mean different things.

## 1. Create the `k3d-fundament-plugin` sandbox cluster

Every command on this page runs from the **repository root**. The sandbox recipes live in
`plugins/mod.just`, which the root `Justfile` registers as the `plugins` module — hence the
`plugins` prefix:

```bash
just plugins cluster-create
just plugins dev
```

The prefix is not cosmetic. `just plugins dev` deploys the plugin-controller into
`k3d-fundament-plugin` and then watches for changes; a bare `just dev` is the root recipe and
deploys the whole platform to `k3d-fundament`.

Like the platform one it passes `--cleanup=false`, so once the controller is available you can
stop it with Ctrl-C and everything stays deployed. Keep it running only while you are editing
plugin-controller source.

## 2. Bridge the two clusters

```bash
just plugins sandbox-orgapi
```

This lets the controller reach organization-api to fetch plugin definitions.

The second bridge is what makes the console real: locally kube-api-proxy runs in `mock` mode and
serves an in-memory fake cluster, and this replaces that mock with a proxy to the real sandbox.
It is a root recipe, so it takes no prefix:

```bash
just plugin-sandbox-kubeconfig
```

That writes a Secret holding a kubeconfig for the sandbox, restarts the two consumers that
mount it, and waits until one of them confirms the switch. It ends with:

```
- kube-api-proxy is proxying to the sandbox (mock disabled)
```

If you do not see that line the bridge is not in place, and plugin pages in the console will
fail to load.

Re-run both bridges after recreating either cluster.

## 3. Publish the plugin definition

A plugin is installed by reference: you publish its **definition** to organization-api, then
point a `PluginInstallation` at it:

```bash
export PLUGIN_REGISTRY=localhost:5112
export FUNDAMENT_ORG_API_URL=https://organization.fundament.localhost:8443
export FUNDAMENT_ORGANIZATION_ID=019b4000-0000-7000-8000-000000000000   # seeded "system" org
export FUNDAMENT_TOKEN=$(./deploy/k3d/dev-token.sh)
just plugins publish cert-manager
```

Expect a last line like:

```
published plugin=cert-manager version=… hash=sha256:… id=… definition_id=…
```

`dev-token.sh` logs in as `platform-admin@fundament.io`. That identity matters: publishing
requires admin of the organization that owns the plugin, first-party plugins are owned by the
seeded `system` org, and the ordinary dev logins are not members of it. Republishing the same
version needs `--replace`.

## 4. Install it

`pluginVersion` and `definitionHash` must both match what step 3 printed — the version is the
plugin's own (`metadata.version` in its `definition.yaml`), not a Fundament version, and it
differs per plugin. Copy both from that output rather than from here:

```bash
kubectl --context k3d-fundament-plugin apply -f - <<'YAML'
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: system--cert-manager
spec:
  definitionRef:
    organizationName: system
    pluginName: cert-manager
    pluginVersion: PASTE_THE_VERSION_FROM_STEP_3
    definitionHash: PASTE_THE_HASH_FROM_STEP_3
YAML
```

The `PluginInstallation` goes in the **sandbox**, not the platform. `definitionHash` is the
admin's record of exactly which definition was approved.

A plugin's identity is the pair `(organizationName, pluginName)`, so `metadata.name` must equal
`<organizationName>--<pluginName>` — here `system--cert-manager`, because first-party plugins are
owned by the seeded `system` org. See
[FUN-17](/funs/fun-17#plugin-identity-and-naming).

:::caution[Always set `definitionHash`, even locally]
The sandbox sets `pluginController.allowUnpinnedHash: true`, which suggests the hash can be
omitted. It cannot. The CRD marks `definitionRef` immutable with a `self == oldSelf` rule, and an
absent `definitionHash` does not survive the controller's round-trip, so its very first update is
rejected:

```
add finalizer: PluginInstallation … spec.definitionRef: Invalid value: definitionRef is immutable once set
```

The CR is created but never reconciles: `PHASE` stays empty and no namespace appears. Delete it,
re-apply with the hash, and it proceeds.

:::

## 5. Watch it come up

```bash
just plugins status
just plugins logs system--cert-manager
```

Expect `PHASE=Running` and `READY=true`. The first install pulls the cert-manager images and
takes a few minutes; the controller polls plugin status every 30 seconds, so the CR lags a
little behind reality.

```bash
kubectl --context k3d-fundament-plugin get plugininstallations
kubectl --context k3d-fundament-plugin -n cert-manager get pods
```

## 6. Prove it works

```bash
just plugins cert-manager test
```

This creates a self-signed `ClusterIssuer` and a `Certificate` and waits for cert-manager to
issue it. Clean up with `just plugins cert-manager test-cleanup`.

## 7. See it in the console

Open <https://console.fundament.localhost:8443> and sign in as `alice@acme-corp.com` with
password `password`. Go to **Clusters → acme-cluster → cert-manager → ClusterIssuers**.

A blank or erroring panel here almost always means step 2 was skipped, or a cluster was recreated
after the bridges were made.

## Clean up

```bash
just plugins uninstall system--cert-manager
just plugins cluster-delete
```

## Known issues on this path

### A platform redeploy invalidates published plugin definitions

`charts/fundament/values-local.yaml` sets `db.reset: true`, so every `just dev` at the repository
root re-seeds the platform database and drops every published `PluginDefinition`.

`just plugins status` will not show it: the `PluginInstallation` keeps reporting `Running` and
`READY=true`, because that status reflects the plugin's own health rather than the controller's
ability to resolve its definition. It surfaces in the controller log:

```
fetch definition: GetPluginDefinition RPC: not_found: plugin definition cert-manager@… not found
```

**Recovery:** redo step 3, then **delete and re-apply** the `PluginInstallation` with the newly
printed hash. It cannot be edited in place — the whole of `definitionRef` is immutable, so
`kubectl apply`/`edit` is rejected with `definitionRef is immutable once set`.

Deleting runs the plugin's uninstall path, so whatever it manages goes with it — for
cert-manager that means cert-manager itself leaves the sandbox and is reinstalled by the new CR.

The hash covers the published manifest, and `publish` injects the freshly built image's
digest into that manifest before hashing. So the hash only stays the same if the rebuild
reproduces a byte-identical image — which a warm Docker cache does, and a cold one does not.
Observed: the same `definition.yaml` published as `sha256:7c9dcb61…` from a warm cache and
`sha256:782914be…` after `docker system prune -a`. Do not assume it is stable.


### `permission_denied` when publishing after a redeploy

The same reset recreates the OpenFGA store, and the authorization tuples written earlier can be
left behind in the previous one. Every authorization decision then resolves to false while every
pod looks healthy — including when the seeded admin row and the tuples themselves look correct.
Restarting `organization-api`, `kube-api-proxy` or `authz-worker` does **not** repair it.

**Recovery:** redeploy the platform again, then republish.


## Other plugins

Steps 3 to 6 work for any plugin that has a catalog entry — only the name and version change.
Steps 1 and 2 are one-time setup, and `ceph-rook` is the exception: it needs block devices on
the node and has its own verification procedure.

`publish` takes a **path** under `plugins/`, while `pluginName` in the CR is the
`metadata.name` from that plugin's `definition.yaml`. They differ for the gateway plugin.

| Plugin (`pluginName`) | `publish` argument | Notes |
|---|---|---|
| `cert-manager` | `cert-manager` | This walkthrough. Has `just plugins cert-manager test` |
| `openfsc` | `openfsc` | Extra setup first; see `plugins/openfsc/README.md` |
| `gateway-api-envoy` | `gateway-api/envoy-gateway` | — |
| `ceph-rook` | `storage/ceph-rook` | **Different procedure** — needs raw block devices and its own baseline checks. Follow the [Ceph/Rook runbook](./ceph-rook-runbook.md), not these steps |
| `external-dns` | — | **Not installable today** — see below |
| `gateway-api-istio` | — | **Not installable today** — same reason as `external-dns` |

Each plugin's version is in its own `definition.yaml`, and `publish` prints it.

`external-dns` and `gateway-api-istio` have source and a `definition.yaml`, but neither is
registered as a module in `plugins/mod.just` nor present in `db/seed/0101-appstore-catalog.sql`.
So `just plugins external-dns test` does not resolve, and `just plugins publish` on either fails
with `no catalog entry`.

