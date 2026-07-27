# Envoy Gateway plugin — design

**Date:** 2026-07-27
**Status:** Approved for planning

## Goal

Add a second Gateway API implementation to Fundament, backed by [Envoy Gateway](https://gateway.envoyproxy.io), alongside the existing Istio-backed `gateway-api` plugin. The plugin must be **idiomatic to Envoy Gateway** — follow how Envoy Gateway actually works, not port Istio-specific mechanics.

A key realization drove the architecture: **plugin `FUNP_*` config is not user-facing.** The install modal (`console-frontend/src/app/install-plugin-modal`) only lets a user pick clusters; the `PluginDefinition` has no config-schema field and there is no edit-config UI. So baking Gateway/TLS/service-type choices into plugin env vars would hide them from users entirely. Instead, per-resource configuration belongs in the console's CRD UI. The plugin installs and runs the **platform**; users create and configure **resources** through the console.

## Restructure

Both backends live under `plugins/gateway-api/` as sibling plugins:

```
plugins/gateway-api/
  istio/          # everything currently in plugins/gateway-api/ moves here, logic unchanged
    config.go  gateway.go  istio.go  plugin.go  console.go  main.go
    Dockerfile  definition.yaml  *_test.go  console/
  envoy-gateway/  # new plugin
    config.go  envoygateway.go  plugin.go  console.go  main.go
    Dockerfile  definition.yaml  *_test.go
    console/      # built assets (embedded)
    console-ui/   # Vite/TS source for the custom Gateway create form (built into console/)
```

All plugins share the repo-root `go.mod`, so relocating the Istio files is just a move of `package main` files.

### Changes required by the Istio move

- `Dockerfile`: build path `./plugins/gateway-api` → `./plugins/gateway-api/istio`; update the `definition.yaml` COPY path.
- `definition.yaml`: `metadata.name` `gateway-api` → `gateway-api-istio`; `displayName` → `Gateway API (Istio)`.
- `Justfile` recipes address plugins by nested path, e.g. `just plugin-publish gateway-api/istio` and `just plugin-publish gateway-api/envoy-gateway`. Confirm `plugin-logs`/`plugin-uninstall` still key off `metadata.name`.
- The Istio plugin's Go logic is otherwise unchanged. Its existing default-Gateway behavior is **out of scope** for this change (not migrated to the new model here).

## Backend plugin (Go) — install & run the platform

Same lifecycle shape as the Istio plugin (`Start`/`Install`/`Uninstall`/`Upgrade`/`Reconcile`), but the plugin only manages the platform, never a default Gateway.

### Installation

- Single Helm **OCI** chart: `oci://docker.io/envoyproxy/gateway-helm`, release `eg`, namespace `envoy-gateway-system`, version pinned via config (default `v1.8.3`).
- The chart bundles the standard Gateway API CRDs **and** the Envoy Gateway CRDs — no separate CRD install step.
- The data plane (Envoy proxy fleet) is auto-provisioned by the controller per `Gateway`. There is no separate "gateway" chart like Istio's.

**Shared-helper addition.** The `plugin-sdk/.../helpers/helm` client has `Install` (no `--version`) and `InstallFromRepo` (uses `--repo`, wrong for OCI). Add:

```go
// InstallFromOCI runs "helm upgrade --install <release> <ociRef>" pinning --version.
func (c *Client) InstallFromOCI(ctx context.Context, releaseName, chartRef, version string, values map[string]string) error
```

Reuses `runInstall` (RBAC-forbidden retry) and `appendSortedValues`.

### Ensure order & lifecycle

1. **Install chart** (controller into `envoy-gateway-system`).
2. **Ensure `GatewayClass`** (`eg`) with `spec.controllerName: gateway.envoyproxy.io/gatewayclass-controller`. This is the one cluster-singleton prerequisite worth auto-creating so users don't hand-author `controllerName`. No `parametersRef` — data-plane infra (service type, replicas) is configured per-need via user-created `EnvoyProxy`/`Gateway.spec.infrastructure`, not baked in.
3. **Health-check**: `envoy-gateway` Deployment in `envoy-gateway-system` has `availableReplicas > 0` (same unstructured pattern as the Istio plugin's `istiod` check).

- **No** default Gateway, **no** env-configured `EnvoyProxy`, **no** cert-manager auto-annotation.
- **Reconcile** re-verifies CRDs, checks controller health, re-ensures the `GatewayClass`.
- **CRD verify** list: the 5 standard Gateway API CRDs plus `envoyproxies`, `securitypolicies`, `backendtrafficpolicies`, `clienttrafficpolicies` (`.gateway.envoyproxy.io`).
- **Uninstall**: block if user-created Gateways/Routes remain, then delete the `GatewayClass` the plugin created, then `helm uninstall eg`. Envoy policy CRs are not part of the guard (harmless once the controller is gone).

### Config (`FUNP_*` env vars) — operator-level only

| Env var | Default | Purpose |
|---|---|---|
| `FUNP_ENVOY_GATEWAY_VERSION` | `v1.8.3` | Pinned `gateway-helm` chart version |
| `FUNP_GATEWAY_NAMESPACE` | `envoy-gateway-system` | Namespace for the controller |
| `FUNP_GATEWAY_CLASS_NAME` | `eg` | GatewayClass name to ensure |

### Why not copy Istio's mechanics

- **No `IstioProfile`** (mesh sidecar-injection toggle — no Envoy Gateway analog).
- **No env-configured default Gateway** — per-resource config belongs in the console CRD UI, not hidden env vars (see Goal).
- Status signals use the standard Gateway API `Accepted` conditions, not Istio-specific fields.

## Frontend — console surface

The console already provides generic CRD UI under `console-frontend/src/app/plugin-resources/`:

- **`resource-list`** and **`resource-detail`** have generic, schema-driven fallbacks — any CRD declared in the menu gets a list table and detail view for free.
- **`resource-create`** has **no** generic fallback: without a plugin-shipped custom component it shows "Creating this resource from the console is not available." Custom create UIs are HTML/TS assets the plugin ships in `console/`, declared via `customComponents.<Kind>.create`, rendered in a sandboxed iframe on the plugin-proxy origin. The iframe SDK (`window.fundament.init` / `window.fundament.k8s.create`) talks to kube-api-proxy directly with a minted token. openfsc's `console-ui/` is the reference implementation.

### definition.yaml (console surface)

- `metadata.name: gateway-api-envoy`, `displayName: Gateway API (Envoy Gateway)`, tags `networking`/`gateway`/`envoy`/`ingress`, homepage `https://gateway.envoyproxy.io`.
- **RBAC** (materialized verbatim into the plugin SA's scope ClusterRole; mirrors the Istio plugin's Helm-install model — if sandbox testing surfaces a missing install permission, add it here, don't special-case in code):
  - `gateway.networking.k8s.io`: `gateways`, `httproutes`, `grpcroutes`, `tcproutes`, `tlsroutes` (full verbs) + their `/status` (get/update).
  - `gateway.networking.k8s.io`: `gatewayclasses` full verbs + `gatewayclasses/status` get/update.
  - `gateway.envoyproxy.io`: `*` get/list/watch, plus full verbs on `envoyproxies`, `securitypolicies`, `backendtrafficpolicies`, `clienttrafficpolicies`.
  - `""`: `secrets`, `namespaces` get/list/watch.
  - `capabilities: [internet_access]`.
- **Menu** (`project` + `organization`): Gateway, HTTPRoute, GRPCRoute, TCPRoute, TLSRoute, SecurityPolicy, BackendTrafficPolicy, ClientTrafficPolicy (all `list: true, detail: true`). `EnvoyProxy` is not surfaced (infra-level).
- **customComponents:** `Gateway: { create: gateways-create.html }` (v1 — the guided create form). No other custom components in v1.
- **uiHints statusMapping:** standard resources reuse the Istio plugin's `Accepted`-condition mappings; the 3 policies map their accepted/programmed status condition to success/danger/pending.

### Custom Gateway create form (`console-ui/`)

Built with Vite/bun (mirroring openfsc), output embedded into `console/`. Uses the `@nldd` design system web components and the plugin iframe SDK.

Guided form with sensible pre-filled defaults:

- **Name** (required) and **Namespace** (dropdown from the host-provided project namespaces, or free-text at org level).
- **`gatewayClassName`**: `eg` (fixed/hidden — the plugin's class).
- **HTTP listener**: port 80, `allowedRoutes.namespaces.from: All` — always included.
- **HTTPS listener** (toggle): port 443, `tls.mode: Terminate`; user supplies a TLS secret name **or** enables a cert-manager cluster-issuer annotation (issuer name input) so cert-manager provisions the secret.
- Submits the built `Gateway` body via `window.fundament.k8s.create({group:'gateway.networking.k8s.io', version:'v1', resource:'gateways', namespace}, body)`, then navigates to the resource detail.
- Form logic kept DOM-lookup-free and unit-tested (as openfsc's `form.ts` is).

## Testing

- **Go** (`testify` `assert`/`require`): install invocation shape (OCI ref, version, values); config defaults/overrides; `ensureGatewayClass` body (controllerName); CRD-verify list; uninstall guard (blocks when user Gateways/Routes exist, allows when only plugin-created resources remain).
- **console-ui** (`bun`/vitest, mirroring openfsc `form.test.ts`): Gateway body building — HTTP-only vs HTTPS with TLS secret vs HTTPS with cert-manager annotation; validation.

## v1 scope

- Custom **create** UI for **Gateway** only. Routes and policies get generic **list + detail**; their **create** stays "not available in console" (kubectl/GitOps) as a documented fast-follow.
- No EnvoyProxy surface in v1 — it is not in the menu (no list/detail/create). Users create `EnvoyProxy` via kubectl when they need non-default data-plane infra. RBAC still grants access so a fast-follow can add it to the menu.

## Out of scope

- No refactor of the Istio plugin's logic beyond the directory move + rename; its default-Gateway behavior is left as-is.
- Custom create forms for routes/policies (fast-follow).
- Per-Gateway service-type UI (fast-follow, via `EnvoyProxy` + `Gateway.spec.infrastructure.parametersRef`).
