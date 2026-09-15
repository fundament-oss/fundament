---
title: Testing plugins locally
sidebar:
  order: 4
---

Run a plugin against the local platform: the platform stores its definition, a sandbox cluster runs it, the console shows it.

```
 k3d-fundament (platform)              k3d-fundament-plugin (sandbox)
┌─────────────────────────────┐       ┌─────────────────────────────┐
│ marketplace-registry-api    │◄──────┤ just plugins publish        │
│                             │       │                             │
│ marketplace-catalog-api     ├──────►│ plugin-controller           │
│                     relay :38080    │   runs plugin-<org>--<name> │
│ kube-api-proxy              │       │                             │
│ plugin-proxy                ├──────►│ (console reads the sandbox) │
│             Secret plugin-sandbox-kubeconfig                      │
└─────────────────────────────┘       └─────────────────────────────┘
 registry localhost:5111               registry localhost:5112
```

- Requires the platform from [Getting started](../fundament/getting-started.md). Run the `just` commands from the fundament repository root.
- [Example: cert-manager](#example-cert-manager) runs every step for one plugin.
- Plugin-specific steps (extra images, `spec.config`, test recipes) are in each plugin's `plugins/<path>/README.md`.

## 1. Create the sandbox

```shell
just plugins cluster-create
just plugins deploy
```

- `cluster-create` creates `k3d-fundament-plugin` and switches the kubectl context to it.
- `deploy` builds plugin-controller from source and deploys it.

## 2. Connect the clusters

```shell
just plugin-sandbox-kubeconfig
just plugins sandbox-catalog
```

```text
- kube-api-proxy is proxying to the sandbox (mock disabled)
- marketplace-catalog-api reachable from the sandbox at http://host.k3d.internal:38080
```

## 3. Log in with functl

In the console, log in as an admin of the publishing organization and create a key under **API keys** ([API keys](../../user/api-keys.md)). Then:

| Organization | ID | Admin |
|---|---|---|
| `system` (first-party plugins) | `019b4000-0000-7000-8000-000000000000` | `platform-admin@fundament.io` |
| `acme-corp` | `019b4000-0000-7000-8000-000000000001` | `alice@acme-corp.com` |

**Plugin in this repository**: `just functl` uses the local endpoints from `mise.toml`.

```shell
just functl auth login <API_KEY>
just functl org set <organization-id>
```

**Plugin in its own project**: point functl at the local endpoints; without them it talks to `fundament-poc.nl`.

```shell
export FUNCTL_API_ENDPOINT=https://organization.fundament.localhost:8443
export FUNCTL_AUTHN_URL=https://authn.fundament.localhost:8443
export FUNCTL_REGISTRY_URL=https://marketplace-registry-api.fundament.localhost:8443
functl auth login <API_KEY>
functl org set <organization-id>
```

## 4. Publish

**Plugin in this repository**:

```shell
PLUGIN_REGISTRY=localhost:5112 just plugins publish <path>
```

- `<path>` is the plugin directory under `plugins/`, e.g. `gateway-api/envoy-gateway`.
- Builds the image with the repository root as build context, pushes it and publishes `plugins/<path>/definition.yaml`.
- Extra flags go to `functl plugin publish`.

**Plugin in its own project**, from the project directory (names from the scaffolded `my-plugin`):

```shell
just docker v0.1.0
docker tag my-plugin-plugin:v0.1.0 localhost:5112/my-plugin:v0.1.0
docker push localhost:5112/my-plugin:v0.1.0
functl plugin publish definition.yaml --image=localhost:5112/my-plugin@sha256:<digest from push> --create
```

Both print:

```text
published plugin=<name> version=<version> hash=sha256:… id=… version_id=… status=SUBMISSION_STATUS_DRAFT
```

- `--create` reserves the listing on the first publish; without it functl answers `no listing named "<name>" in this organization — pass --create to reserve it`.
- `--submit` opens the review: see [Marketplace route](#marketplace-route).
- Step 5 needs `version` and `hash`.
- Versions are create-only: bump `metadata.version` to publish changed content.

## 5. Install

```shell
kubectl --context k3d-fundament-plugin apply -f - <<'YAML'
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: <organizationName>--<pluginName>
spec:
  definitionRef:
    organizationName: <organizationName>
    pluginName: <pluginName>
    pluginVersion: "<version from step 4>"
    definitionHash: "<hash printed by publish>"   # includes the sha256: prefix
YAML
```

- Draft and pending versions install this way.
- The image comes from the published definition; the CR carries none. Field reference: [PluginInstallation CRD](/docs/developer/plugins#plugininstallation-crd).
- `spec.config` sets the plugin's `FUNP_` values without the prefix, e.g. `OPERATOR_IMAGE: <ref>`.
- `definitionRef` is immutable: to change the version, uninstall and install again.

## 6. Verify

```shell
just plugins status
just plugins logs <organizationName>--<pluginName>
```

```text
NAME                     PLUGIN     ORGANIZATION   PHASE     READY
<organization>--<name>   <name>     <organization> Running   true
```

In the console, log in as a member of an organization with a cluster (`alice@acme-corp.com`) and open the project: the plugin's section appears under its display name.

## First-party plugins

Each first-party plugin documents its flow from step 4, its config and its recipes in `plugins/<path>/README.md`: see [`plugins/`](https://github.com/fundament-oss/fundament/tree/master/plugins). ceph-rook: [Example: Ceph Storage (Rook)](example-ceph-rook.md).

## Marketplace route

1. Publish with `--submit` (step 4): the version becomes **Pending review**.
2. https://marketplace-registry.fundament.localhost:8443 (console session) lists it under **My plugins**.
3. https://marketplace-admin.fundament.localhost:8443 (DCIM login): **Review queue** → **Review** → approve.
4. https://marketplace.fundament.localhost:8443 lists the approved plugin.
5. Console **Plugins** page, as a member of an organization with a cluster: install it on the cluster.

## After a platform deploy

Every platform deploy resets its databases: API keys, published definitions and the cluster's ready state are gone. Installed plugins keep running; a new install reports `fetch definition: GetPluginDefinition RPC: not_found: plugin definition not found`. Repeat:

1. `just plugin-sandbox-kubeconfig`
2. step 3 with a new API key
3. step 4. The hash stays the same while the image content is unchanged. If it differs from the installation's `definitionHash`, uninstall and repeat step 5.

## Remove

```shell
just plugins uninstall <organizationName>--<pluginName>
just plugins cluster-delete
```

A plugin whose pod keeps failing takes minutes to uninstall.

## Example: cert-manager

Log in to the console as `platform-admin@fundament.io` (password `password`) and create an API key under **API keys**. Then:

```shell
just plugins cluster-create
just plugins deploy
just plugin-sandbox-kubeconfig
just plugins sandbox-catalog

just functl auth login <API_KEY>
just functl org set 019b4000-0000-7000-8000-000000000000

PLUGIN_REGISTRY=localhost:5112 just plugins publish cert-manager

kubectl --context k3d-fundament-plugin apply -f - <<'YAML'
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: system--cert-manager
spec:
  definitionRef:
    organizationName: system
    pluginName: cert-manager
    pluginVersion: "1.17.2"
    definitionHash: "<hash printed by publish>"   # includes the sha256: prefix
YAML

just plugins status
just plugins cert-manager test
```

- `publish` prints `published plugin=cert-manager version=1.17.2 hash=sha256:…`; the listing exists, so no `--create`.
- `status` shows `system--cert-manager   cert-manager   system   Running   true`; `test` ends with `test-cert   True   test-cert-tls`.
- `just plugins logs system--cert-manager` streams the plugin log: `cert-manager is running`.
- In the console as `alice@acme-corp.com`: **acme-project** → **Cert Manager** → **Certificates** lists `test-cert` in `fundament`, Ready.

Remove:

```shell
just plugins cert-manager test-cleanup
just plugins uninstall system--cert-manager
just plugins cluster-delete
```
