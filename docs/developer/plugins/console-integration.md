---
title: Console integration
sidebar:
  order: 6
---

How a Fundament plugin renders inside the Console: discovery, the iframe boundary, the postMessage SDK, and the Kubernetes call path through kube-api-proxy in mock, sandbox and real mode. Design: [FUN-17 Plugin Authorization](/funs/fun-17).

This document is the architecture reference for anyone working on the Console-plugin boundary. For a plugin author's how-to, start with [Writing a plugin](writing-a-plugin) and [Custom UI](custom-ui). For the container lifecycle (controller, RBAC, install/uninstall), see the [Plugins overview](.).

## Overview

- **Console frontend** (`console-frontend/`): the host Angular app. Discovers installed plugins, renders the sidebar and routes, mounts plugin iframes and mints PluginTokens for them.
- **Plugin SDK** (`console-frontend/src/plugin-sdk/`): `plugin-sdk.js` and `plugin-sdk.css`, loaded by plugin pages. Handles the postMessage protocol, the PluginToken and Kubernetes calls.
- **plugin-proxy** (`plugin-proxy/`): serves plugin pages and the SDK on its own origin.
- **authn-api** (`authn-api/`): mints PluginTokens.
- **kube-api-proxy** (`kube-api-proxy/`): the gateway for every cluster request, from the console and from plugin pages. Runs in mock, sandbox or real mode.
- **Plugin runtime** (`plugin-sdk/pluginruntime/`): the Go framework each plugin embeds. Serves the plugin's metadata API and its embedded `console/` assets.

```
 browser
 ┌──────────────────────────────────────────────────────────────┐
 │ console origin                  plugin-proxy origin          │
 │ ┌──────────────────┐ postMessage ┌──────────────────────────┐│
 │ │ console-frontend │◄───────────►│ iframe: console/*.html   ││
 │ └────┬────────┬────┘             │ + /plugins/sdk/v1/ SDK   ││
 │      │        │                  └──────┬─────────────┬─────┘│
 └──────┼────────┼─────────────────────────┼─────────────┼──────┘
        │        │                         │             │
        ▼        ▼                         ▼             ▼
 organization-api  kube-api-proxy ◄──────────── k8s.* calls
 (definitions)     (installations)     plugin-proxy (pages, SDK)
                        │                      │
                        ▼                      ▼
                     cluster ◄──────────── plugin pod (/console/)
```

Plugin pages run on the plugin-proxy origin and never see the Console user's cookies. They call kube-api-proxy with a PluginToken: the user, acting through one plugin installation.

## Discovery and registration

1. The console lists the cluster's installations: `GET <kube-api-proxy>/clusters/<cluster-id>/apis/plugins.fundament.io/v1/plugininstallations`.
2. It keeps installations with `status.phase` `Running` and `status.ready`.
3. For each, it fetches the definition from organization-api: `organization.v1.PluginService/GetPluginDefinition` with organization, plugin and version.
4. `menu` entries appear in the sidebar under the plugin's display name.

The definition advertises:

- `menu`: which CRDs appear at organization and project level.
- `customComponents`: a `Kind` → `{ list?, detail?, create? }` map of files under `console/`. Kinds without an entry get the generated UI.
- `permissions.rbac`: the plugin's ServiceAccount RBAC, which also limits the plugin's page calls.
- `allowedResources`: the resources the plugin's pages read.
- `crds`: the CRDs the plugin manages.

:::caution[TODO]
- Nothing checks `allowedResources`: the console loads it into its plugin registry, and kube-api-proxy does not read it. Enforce it or remove it from the definition.
- Plugin sections load only when a project is opened from the sidebar. Opening or refreshing a project URL directly shows none.
:::

## Routing and rendering

The plugin routes live under `plugin-resources/`, at organization level and under `projects/:id/`:

| Route | Component |
| --- | --- |
| `plugin-resources/:pluginName/:resourceKind` | `ResourceListComponent` |
| `plugin-resources/:pluginName/:resourceKind/create` | `ResourceCreateComponent` |
| `plugin-resources/:pluginName/:resourceKind/:resourceId` | `ResourceDetailComponent` |

- `:pluginName` is the PluginInstallation's `metadata.name` (`<organizationName>--<pluginName>`); `:resourceKind` is `<plural>.<group>`.
- Each component looks up `customComponents.<Kind>.list`, `.detail` or `.create`. If present, it mounts the plugin iframe; if absent, it renders the generated view.
- The list shows a create action only when the kind has a `create` component.

### Generated fallback UI

When a plugin provides no `customComponents` entry for a CRD kind, the console renders a generated view from the CRD's OpenAPI v3 schema (loaded from `apiextensions.k8s.io/v1/customresourcedefinitions`):

- **List**: a table whose columns come from the CRD's `additionalPrinterColumns` (falling back to Name + Age), with a row per object and a link to the detail view.
- **Detail**: object metadata, the `spec` fields rendered from the schema, a `status` section (including a conditions table when present), and a delete action.

The generated UI does not create or edit resources. Ship a custom UI for write actions or a bespoke layout.

## The iframe boundary

### URL construction

The console turns the file from `customComponents` into the iframe `src`:

```
https://<plugin-proxy>/clusters/<cluster-id>/plugins/<installation-name>/<version>/console/<file>
```

- plugin-proxy checks the user's access to the cluster, confirms the installed version, and fetches the file from the plugin pod through the cluster's API-server service proxy (`/api/v1/namespaces/<plugin-namespace>/services/<service>/proxy/console/<file>`).
- The response carries `Content-Security-Policy` ([Custom UI](custom-ui#content-security-policy)) and `Cache-Control: private, max-age=31536000, immutable`: a version's assets never change.

### Sandbox

The iframe is created with `sandbox="allow-scripts allow-same-origin allow-forms"`.

- `allow-same-origin` is required: the page runs on the plugin-proxy origin, a different site from the console. The CSP's `script-src 'self'` needs a real origin to resolve, and postMessage target-origin pinning needs a checkable origin at both ends.
- The same-origin policy between sites still blocks access to the console's document, and the user's HttpOnly cookie stays unreachable.
- `allow-forms` serves create pages whose submits stay in the frame. `allow-top-navigation` and `allow-popups` are not granted.

## The postMessage protocol

The SDK sends and handles most messages itself.

### Plugin → host

| Type | When | Payload | Sent by SDK |
| --- | --- | --- | --- |
| `plugin:ready` | The SDK script loads | none | Yes |
| `plugin:resize` | Content height changes (debounced 50 ms, `ResizeObserver`) | `{ height }` | Yes |
| `plugin:request-token-refresh` | A call returned 401 | none | Yes |
| `plugin:navigate` | Open another resource | `{ name, namespace? }` | No: call from your code |
| `plugin:create` | Open the create route | | No |
| `plugin:navigate-back` | Back to the list | | No |

### Host → plugin

| Type | When | Payload |
| --- | --- | --- |
| `fundament:init` | After `plugin:ready`; first message | Init payload, see below |
| `fundament:theme-changed` | User toggles the Console theme | `{ theme: 'light' \| 'dark' }` |
| `fundament:token-refreshed` | The console minted a new PluginToken | `{ token, tokenExpiresAt }` |
| `fundament:auth-failed` | Minting keeps failing | `{ reason: 'mint_failed' \| 'unauthorized' \| 'revoked' }` |

### Init payload fields

| Field | Description |
| --- | --- |
| `protocolVersion` | `1`; the SDK ignores other versions |
| `theme` | `'light'` or `'dark'`; the SDK sets it as a class on `<body>` |
| `pluginName` | The installed plugin's name |
| `crdKind` | The CRD kind being rendered |
| `view` | `'list'`, `'detail'` or `'create'` |
| `resource` | Detail views: `{ name, namespace? }` |
| `namespaces` | Create views in a project: the project's namespaces |
| `kubeApiProxyUrl`, `clusterId` | Base for Kubernetes calls |
| `token`, `tokenExpiresAt` | The PluginToken |

### Origin pinning

The SDK accepts messages only from `window.parent`. The first accepted message must be `fundament:init`; the SDK pins its origin and drops any later message from another origin. Outbound messages that carry nothing sensitive use `'*'` until init arrives; `plugin:request-token-refresh` is sent only to the pinned origin.

### Request lifecycle

- A call waits up to 20 s for a token, then fails.
- Each request is aborted after 30 s and rejects with `SdkError('timeout')`.
- On 401 the SDK drops the token, sends `plugin:request-token-refresh` and retries once; a second 401 rejects with `SdkError('unauthorized')`.
- 403 rejects with `SdkError('forbidden')`, other HTTP errors with `SdkError('http')` carrying the Kubernetes `Status` message, network errors with `SdkError('transport')`.

## The SDK surface

The SDK sets a single global, `window.fundament`:

```ts
interface FundamentSdk {
  init: Promise<InitContext>;
  readonly parentOrigin: string | null;
  getToken(): Promise<string>;
  fetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response>;
  k8s: {
    list<T>(args: { group; version; resource; namespace? }): Promise<{ items: T[] }>;
    get<T>(args: { group; version; resource; name; namespace? }): Promise<T>;
    create<T>(args: { group; version; resource; namespace? }, body: unknown): Promise<T>;
    patch<T>(args: { group; version; resource; name; namespace? }, body: unknown): Promise<T>; // merge patch
    delete<T>(args: { group; version; resource; name; namespace? }): Promise<T>;
  };
  onThemeChange(cb: (theme: 'light' | 'dark') => void): () => void;
}
```

The SDK also, on its own:

- Applies `light`/`dark` as a class on `<body>` on `fundament:init` and on every `fundament:theme-changed`.
- Reports `plugin:resize` once stylesheets have loaded and on every `ResizeObserver` callback.
- Pins the parent origin and attaches and refreshes the PluginToken.

plugin-proxy serves the bundle at `/plugins/sdk/v1/plugin-sdk.js` (and `.css`); the Console serves the same files at `/plugin-ui/`.

## How a plugin loads the SDK

The page CSP allows scripts and styles only from the page's own origin, so pages load the SDK from plugin-proxy at `/plugins/sdk/v1/`. The cert-manager plugin's `_shared.js` is the reference implementation:

```js
export function loadSdk() {
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = '/plugins/sdk/v1/plugin-sdk.css';
  document.head.appendChild(link);

  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = '/plugins/sdk/v1/plugin-sdk.js';
    script.onload = () => resolve(window.fundament);
    script.onerror = () => reject(new Error('failed to load plugin-sdk.js'));
    document.head.appendChild(script);
  });
}
```

Every cert-manager page script starts with `await loadSdk(); await fundament.init;` and then fetches its data through `fundament.k8s.list` / `.get`.

## Kubernetes call path

For a `fundament.k8s.list({ group: 'cert-manager.io', version: 'v1', resource: 'certificates' })` call from inside the iframe:

1. The console mints a PluginToken for the user and the installation (authn-api `MintPluginToken`: 15 minutes, `aud=fundament-plugin`) and sends it in `fundament:init`.
2. The SDK builds `<kubeApiProxyUrl>/clusters/<cluster-id>/apis/<group>/<version>/[namespaces/<ns>/]<resource>` (`/api/<version>/…` for core resources) and calls it with `Authorization: Bearer <PluginToken>`.
3. kube-api-proxy checks, in order:
   1. the token
   2. the cluster in the token matches the path
   3. OpenFGA `can_view` on the cluster
   4. a SubjectAccessReview for the user's ServiceAccount `fundament-system/fundament-<user-id>`
4. It forwards the call with a token for the plugin's ServiceAccount: the cluster's RBAC from `permissions.rbac` decides. The effective permission is the intersection of the user's and the plugin's.
5. It logs a `plugin gateway request` with user, installation, plugin, version, definition hash and decision.
6. The SDK resolves the promise with the response, or rejects with an `SdkError`.

:::caution[TODO]
The local sandbox does not apply step 4: kube-api-proxy's sandbox proxy builds its transport from the sandbox kubeconfig, and that kubeconfig's admin client certificate authenticates the call before the plugin's Bearer token. Page calls get the admin's access. Until this is fixed, check a plugin's RBAC with `kubectl --context k3d-fundament-plugin auth can-i <verb> <resource> --as=system:serviceaccount:plugin-<installation-name>:plugin`.
:::

## kube-api-proxy: mock, sandbox and real

### Shared behavior

- `/clusters/<cluster-id>/{api|apis|openapi|version}/…` is forwarded to the cluster handler; other roots return 404.
- A PluginToken takes the gateway path above.
- A UserToken or the console's cookie takes the user path: token validation, OpenFGA `can_view` on the cluster, then the call is forwarded.

### Mock mode

`KUBE_API_PROXY_MODE=mock` (default) answers from fixtures:

- **Resources**: cert-manager (Certificates, CertificateRequests, Issuers, ClusterIssuers), CloudNativePG (Databases, Backups, Subscriptions), `demo.fundament.io` DemoItems and OpenFSC FSCInstallations (including create).
- **PluginInstallations**: `GET`, `POST` and `DELETE`, held in memory per cluster. A restart loses them.
- **PluginToken path**: the user SubjectAccessReview allows all, and the plugin ServiceAccount token is a placeholder.

:::caution[TODO]
Mock mode still serves `/proxy/console/<file>` from `MOCK_PLUGIN_TEMPLATES_DIR` without authentication. The console loads plugin pages from plugin-proxy, so this path is unused: remove it.
:::

### Sandbox mode

`just plugin-sandbox-kubeconfig` sets `PLUGIN_SANDBOX_KUBECONFIG`, which switches mock mode to the `k3d-fundament-plugin` cluster:

- **User path**: forwarded with the sandbox kubeconfig's credentials.
- **PluginToken path**: SubjectAccessReview for the user against the sandbox, then the plugin ServiceAccount token (see the TODO under [Kubernetes call path](#kubernetes-call-path)).

### Real mode

`KUBE_API_PROXY_MODE=real` with `GARDENER_KUBECONFIG`:

- **Clusters**: the proxy fetches each shoot's admin kubeconfig from Gardener, caches it and refreshes it at 70 % of its TTL.
- **User path**: each request uses a token for the user's ServiceAccount `fundament-system/fundament-<user-id>`, requested through the TokenRequest API, cached per user and cluster, refreshed at 80 % of its TTL, with concurrent requests deduplicated. Before the ServiceAccount exists the proxy answers `503 service account sync pending`.
- **PluginToken path**: SubjectAccessReview for the user on the shoot, then the plugin ServiceAccount token.

### Implications

- **Console work without a cluster**: mock mode; the fixtures cover the resources above.
- **Plugin runtime and plugin pages**: sandbox mode. The plugin's own container runs, its pages load from plugin-proxy, and the metadata API is answered by the plugin.
- **Shoot clusters and per-user ServiceAccounts**: real mode only.

### Local dev shortcuts

```bash
just dev-hotreload              # mock mode
just plugin-sandbox-kubeconfig  # sandbox mode, see Local development
just dev -p local-gardener      # real mode against a local Gardener
```

## Plugin author's quick guide

1. **Declare it in `definition.yaml`**: map your HTML files to CRD kinds in `spec.customComponents`, and give the plugin the RBAC its pages need in `spec.permissions.rbac`.
2. **Embed the assets**: put your HTML/JS/CSS under `console/` and return them from `ConsoleAssets()`:

   ```go
   //go:embed console/*
   var consoleFiles embed.FS

   func (p *MyPluginPlugin) ConsoleAssets() http.FileSystem {
       return console.NewFileSystem(consoleFiles, "console")
   }
   ```

3. **Load the SDK from plugin-proxy**: `/plugins/sdk/v1/plugin-sdk.js` and `.css`, with a `.js` module per page: the CSP blocks inline scripts. Copy `loadSdk()` from the cert-manager plugin's `_shared.js`.
4. **Render**: `await fundament.init` for the context, call `fundament.k8s.*` for data, and post `plugin:navigate` to open a detail view.

## Verifying the integration end-to-end

When changing anything on the Console-plugin boundary, walk through the full path in sandbox mode:

1. Install a plugin with custom pages as in [Testing plugins locally](testing-plugins-locally), then open a project with a cluster from the sidebar and open the plugin's section.
2. In browser devtools:
   - The iframe `src` is on plugin-proxy and carries the installation name and version; the response has the plugin CSP.
   - The iframe posts `plugin:ready` and the console answers with `fundament:init`.
   - Data requests go to kube-api-proxy with `Authorization: Bearer`.
   - Clicking a row posts `plugin:navigate` and the detail view loads.
3. `kubectl --context k3d-fundament -n fundament logs deploy/kube-api-proxy` shows a `plugin gateway request` per call with its decision.
4. **Authorization spot-check**: call a resource outside the plugin's `permissions.rbac`. The call should reject with `SdkError('forbidden')`; in the sandbox it succeeds until the TODO under [Kubernetes call path](#kubernetes-call-path) is fixed.
