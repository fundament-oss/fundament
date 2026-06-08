---
title: Console integration
sidebar:
  order: 5
---

How a Fundament plugin renders inside the Console: the iframe boundary, the
postMessage SDK, the Kubernetes-call broker, and the role of the kube-api-proxy
in mock and real mode.

This document is the architecture reference for anyone working on the
Console-plugin boundary. For a plugin author's how-to, start with
[Writing a plugin](writing-a-plugin) and [Custom UI](custom-ui). For the
container lifecycle (controller, RBAC, install/uninstall), see the
[Plugins overview](.).

## Overview

Four moving parts collaborate to render a plugin's UI in the Console:

- **Console frontend** (`console-frontend/`) — the host Angular app. Discovers
  installed plugins, renders the sidebar, mounts plugin iframes, and brokers
  Kubernetes API calls.
- **Plugin SDK** (`console-frontend/src/plugin-sdk/`) — a tiny TypeScript
  library compiled to `plugin-sdk.js` / `plugin-sdk.css` and served by the
  Console at `/plugin-ui/`. Plugins load it inside their iframe to talk to the
  host.
- **kube-api-proxy** (`kube-api-proxy/`) — the gateway every cluster request
  goes through. Has a **mock** mode (in-memory fixtures, used for local dev)
  and a **real** mode (Gardener-managed shoots, used in staging/prod).
- **Plugin runtime** (`plugin-sdk/pluginruntime/`) — the Go framework each
  plugin embeds. Serves its `PluginMetadataService` and its embedded
  `console/` HTML/JS/CSS assets.

The plugin's UI never runs on the Console's origin and never gets the Console
user's cookies. Everything crosses the iframe boundary as `postMessage` calls,
and every Kubernetes read is brokered by the Console host on behalf of the
logged-in user.

## Discovery and registration

When the Console boots for a cluster, `PluginRegistryService.loadPlugins()`:

1. Fetches the cluster's `PluginInstallation` list:
   `GET /clusters/{clusterId}/apis/plugins.fundament.io/v1/plugininstallations`.
2. Keeps only items whose `status.phase === 'Running' && status.ready`.
3. For each running plugin, calls the plugin's
   `PluginMetadataService.GetDefinition` Connect RPC through the Kubernetes
   apiserver service proxy:
   `GET /clusters/{clusterId}/api/v1/namespaces/plugin-{name}/services/http:plugin-{name}:8080/proxy/pluginmetadata.v1.PluginMetadataService/GetDefinition`.
4. Stores the parsed definitions in a signal that the sidebar and routes
   subscribe to.

The definition advertises:

- `menu` — which CRDs appear at organization and project level.
- `customComponents` — a `Kind` → `{ list?, detail? }` map of relative paths
  (e.g. `Certificate` → `certificates-list.html`). Plugins are expected to
  provide custom components for every CRD they expose in the menu; the
  console does not generate fallback views.
- `allowedResources` — the Kubernetes resources the plugin's iframe may read,
  with explicit verbs (`get`, `list`). This is the authoritative list the host
  enforces on every K8s broker call.
- `crds` — the CRDs the plugin manages.

## Routing and rendering

The plugin routes live under `/plugin-resources/` (also mirrored under
`/projects/:id/plugin-resources/`):

| Route | Component |
| --- | --- |
| `/plugin-resources/:pluginName/:resourceKind` | `ResourceListComponent` |
| `/plugin-resources/:pluginName/:resourceKind/:resourceId` | `ResourceDetailComponent` |

Each component looks up the matching `customComponents.<Kind>.list` or
`.detail` entry in the plugin definition, builds the iframe URL, and
mounts the iframe component pointed at it. A plugin that omits a custom
component for one of its CRDs will render an empty page — the console
does not generate a fallback view from the CRD schema, so every CRD
exposed in the menu needs its own `list` and `detail` HTML.

## The iframe boundary

### URL construction

The console turns the relative path from `customComponents` into the
iframe `src`:

```
{kubeApiProxyUrl}/clusters/{clusterId}
  /api/v1/namespaces/plugin-{name}
  /services/http:plugin-{name}:8080
  /proxy/console/{file}?host={consoleOrigin}
```

Two things to note:

- The asset is fetched through the kube-api-proxy and then through the
  apiserver's service proxy — never directly from the plugin's pod. In real
  mode that means the apiserver tunnels to the plugin's HTTP server (port
  8080, `/console/<file>`); in mock mode the kube-api-proxy serves the file
  from disk and never talks to a pod.
- `host={consoleOrigin}` is appended so the iframe can load the SDK
  (`plugin-sdk.js`/`.css`) from the Console origin. The iframe itself lives
  on the kube-api-proxy origin and has no other way to learn where to fetch
  the SDK from.

Absolute paths and paths starting with `/plugin-ui/` are passed through
unchanged.

### Sandbox

The iframe is created with `sandbox="allow-scripts"` and nothing else.
`allow-same-origin` is **deliberately** omitted, which has two consequences
plugin authors must keep in mind:

1. The iframe runs with an opaque origin. It cannot send Console cookies or
   read the Console's storage. It also cannot do its own credentialed `fetch`
   against the kube-api-proxy.
2. All cluster data must therefore flow through the host-mediated broker
   (`plugin:k8s:list` / `plugin:k8s:get`). This is the security boundary: the
   host validates every request against `allowedResources` before forwarding.

## The postMessage protocol

The reference SDK takes care of most messages automatically.

### Plugin → host

| Type | When | Payload | Sent by SDK |
| --- | --- | --- | --- |
| `plugin:ready` | Immediately after the SDK loads. | _(none)_ | Yes — auto. |
| `plugin:resize` | Content height changes (debounced 50 ms; tracked via `ResizeObserver`). | `{ height: number }` | Yes — auto. |
| `plugin:navigate` | Plugin wants Console to navigate to another resource. | `{ name: string, namespace?: string }` | No — call from your code. |
| `plugin:k8s:list` | `fundament.k8s.list(args)` is called. | `{ requestId, group, version, resource, namespace? }` | Yes — via SDK. |
| `plugin:k8s:get` | `fundament.k8s.get(args)` is called. | `{ requestId, group, version, resource, name, namespace? }` | Yes — via SDK. |

### Host → plugin

| Type | When | Payload |
| --- | --- | --- |
| `fundament:init` | After `plugin:ready` arrives. First message; carries everything the plugin needs to render. | `{ theme, pluginName, crdKind, view, resource? }` |
| `fundament:theme-changed` | User toggles the Console theme. Watched via a `MutationObserver` on `<html>` class. | `{ theme: 'light' \| 'dark' }` |
| `fundament:k8s:result` | Reply to a `plugin:k8s:list` or `plugin:k8s:get`. Matched by `requestId`. | Success: `{ requestId, ok: true, items?, item? }`. Error: `{ requestId, ok: false, error, status? }`. |

### Init payload fields

| Field | Description |
| --- | --- |
| `theme` | `'light'` or `'dark'`; the SDK applies it as a class on `<body>` automatically. |
| `pluginName` | The installed plugin's name. |
| `crdKind` | The CRD kind being rendered (e.g. `Certificate`). |
| `view` | `'list'` or `'detail'`. |
| `resource` | Only on detail views: `{ name, namespace? }`. |

### Origin pinning

The SDK posts `plugin:ready` with target origin `*` (the host origin is not
yet known). On the **first** incoming message — which must be
`fundament:init` — the SDK captures `event.origin` and refuses any further
message whose origin doesn't match. After that, the SDK targets the
captured origin on outbound messages.

### Request lifecycle

`fundament.k8s.list` / `fundament.k8s.get` generate a `requestId`, post the
`plugin:k8s:*` message, and return a promise. The promise resolves when the
matching `fundament:k8s:result` arrives, rejects with `SdkError` if `ok:
false`, and rejects with `code: 'timeout'` after **10 seconds**.

The host validates each request against the plugin's own `allowedResources`
before forwarding:

- Allowed → host `fetch`es the kube-api-proxy with the user's session cookie
  and forwards the result.
- Not allowed → host replies `{ ok: false, error: 'forbidden' }` and logs a
  warning; the SDK rejects with `SdkError('forbidden', ...)`.

## The SDK surface

The SDK sets a single global, `window.fundament`:

```ts
interface FundamentSdk {
  init: Promise<InitContext>;
  k8s: {
    list<T>(args: { group; version; resource; namespace? }): Promise<{ items: T[] }>;
    get<T>(args:  { group; version; resource; name; namespace? }): Promise<T>;
  };
  onThemeChange(cb: (theme: 'light' | 'dark') => void): () => void;
}
```

The SDK also does these things on its own so plugins don't have to:

- Applies `light`/`dark` as a class on `<body>` on `fundament:init` and on
  every `fundament:theme-changed`.
- Reports `plugin:resize` after stylesheets finish loading, and on every
  `ResizeObserver` callback (debounced 50 ms).
- Pins the parent origin on the first message and validates all subsequent
  messages.

The console serves the compiled bundle at the stable path
`/plugin-ui/plugin-sdk.js` (and `.css`).

## How a plugin loads the SDK

Because the iframe is sandboxed and lives on the kube-api-proxy origin, it
can't statically link to `/plugin-ui/plugin-sdk.js` — that path is on the
Console origin. The Console solves this by appending `?host={consoleOrigin}`
to the iframe URL; plugin pages read it and inject `<script>` / `<link>` tags
themselves.

The cert-manager plugin's `_shared.js` is the reference implementation:

```js
export function hostOrigin() {
  return new URLSearchParams(location.search).get('host') ?? '';
}

export function loadSdk() {
  const host = hostOrigin();
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = `${host}/plugin-ui/plugin-sdk.css`;
  document.head.appendChild(link);

  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = `${host}/plugin-ui/plugin-sdk.js`;
    script.onload = () => resolve(window.fundament);
    script.onerror = () => reject(new Error('failed to load plugin-sdk.js'));
    document.head.appendChild(script);
  });
}
```

Every cert-manager template starts with `await loadSdk(); await fundament.init;`
and then fetches its data through `fundament.k8s.list` / `.get`.

## Kubernetes call path

Putting the pieces together for a `fundament.k8s.list({ group: 'cert-manager.io',
version: 'v1', resource: 'certificates' })` call from inside the iframe:

1. SDK posts `plugin:k8s:list { requestId, group, version, resource }` to
   `window.parent`.
2. The console validates the request against the plugin's
   `allowedResources` (group + version + resource + verb `list`).
3. If allowed, the console builds
   `{kubeApiProxyUrl}/clusters/{clusterId}/apis/{group}/{version}/{resource}`
   (or `/api/{version}/...` for core resources, plus `/namespaces/{ns}` when
   namespaced) and fetches it with the user's session cookie.
4. The kube-api-proxy authenticates the request (JWT cookie), checks
   `can_view` on the cluster via OpenFGA, exchanges that for a 15-minute
   ServiceAccount token (real mode) or skips straight to the in-memory store
   (mock mode), and returns the response.
5. The host posts `fundament:k8s:result { requestId, ok: true, items }` back
   to the iframe. The SDK matches `requestId`, resolves the promise.

The plugin's iframe never sees the user's JWT and never talks directly to the
kube-api-proxy. Everything runs as the **user**, with the user's RBAC — the
plugin's own ServiceAccount is irrelevant to UI reads.

## kube-api-proxy: mock vs real

The proxy supports two modes selected at startup by the
`KUBE_API_PROXY_MODE` env var (default `mock`). Real mode additionally
requires `GARDENER_KUBECONFIG`.

### Shared behavior

Both modes expose the same external surface:

- `/clusters/{clusterID}/{api|apis|openapi/...}` — Kubernetes API proxy, the
  only path forwarded to the cluster handler. Paths outside that allowlist
  return 404.
- `/livez`, `/readyz` — health probes.
- Plugin console asset paths (`/clusters/{id}/api/v1/namespaces/plugin-{name}/services/http:plugin-{name}:8080/proxy/console/...`)
  are treated as **public** static assets: the auth/authz check is skipped
  because the sandboxed iframe runs with an opaque origin and cannot send the
  JWT cookie anyway. The assets themselves carry no secrets — they are
  HTML/JS templates.

All other paths run the full pipeline: JWT validation → OpenFGA `can_view` on
the cluster → (real mode) per-user SA-token exchange → proxy to the cluster
handler.

### Mock mode

In mock mode the proxy answers Kubernetes calls from hardcoded fixtures
instead of talking to a cluster:

- **Resources**: hardcoded JSON for cert-manager, CloudNativePG, and the
  demo plugin. `GET` requests for those groups/versions/resources return
  the fixture; everything else returns an empty list.
- **`PluginInstallation` CRUD**: supports `GET`, `POST`, `DELETE` on
  `/apis/plugins.fundament.io/v1/plugininstallations`. State is held
  in-memory, partitioned per cluster ID.
- **Persistence**: none. Restart loses all installations created through the
  UI and any state written through the proxy.
- **Plugin metadata RPC**: `GetDefinition` calls are answered with hardcoded
  definition JSON. The real plugin binary is not running.
- **Console assets**: served from the local filesystem at
  `${MOCK_PLUGIN_TEMPLATES_DIR}/{pluginName}/console/{asset}` (default
  `./plugins`). Responses include `Cache-Control: no-store` so iframe
  reloads always pick up edits — refresh the browser and your edits to
  `plugins/cert-manager/console/certificates-list.html` are live. CORS is
  overridden to `Access-Control-Allow-Origin: *` (necessary because the
  sandboxed iframe's `Origin: null` is not on the proxy's normal
  allowlist).
- **No Gardener, no OpenFGA, no SA tokens**. JWT validation still runs on
  non-asset paths.

### Real mode

Real mode wires up the full production stack:

- **Cluster discovery**: shoots are looked up in the Gardener hub by label
  `fundament.io/cluster-id={clusterID}`. The admin kubeconfig is fetched
  on-demand and cached with singleflight deduplication; entries refresh at
  70 % of TTL.
- **Per-user authentication**: each request, the proxy fetches a 15-minute
  ServiceAccount token for `fundament-{userID}` in the
  `fundament-system` namespace via the Kubernetes TokenRequest API. Tokens
  are cached per `(userID, clusterID)` and proactively refreshed at 80 % of
  TTL; a 401 from the shoot triggers a forced refresh.
- **Authorization**: every cluster API call requires `can_view` on the
  cluster in OpenFGA. The plugin's own `allowedResources` is a second layer
  enforced client-side in the Console — the real authorization gate is the
  shoot's RBAC on the user's SA, which is what ultimately answers `403`.
- **Console assets**: instead of reading from disk, the proxy forwards the
  `/proxy/console/...` request to the plugin pod's HTTP server (port 8080).
  The plugin's `ConsoleProvider.ConsoleAssets()` serves the embedded
  `console/` filesystem from inside the binary.

### Implications

For **frontend iteration** on a plugin's UI (HTML/CSS/JS edits), prefer mock
mode. The on-disk asset serving with `Cache-Control: no-store` plus the
fixture data gives you a tight reload loop without a running cluster.
Anything that writes state will, however, vanish on restart.

For **plugin runtime work** (install logic, RBAC, Helm steps, status
reporting), use real mode. It is the only mode where the plugin's own
container is actually running, where RBAC is genuinely enforced, and where
the metadata RPC is answered by the plugin instead of a hardcoded fixture.

For **end-to-end tests** that depend on persistence or cross-pod
interaction, only real mode is meaningful.

### Local dev shortcuts

```bash
just dev                    # mock mode (default)
just dev -p local-gardener  # real mode against a local Gardener
```

The hot-reload dev image rebuilds and restarts the binary on every Go
source change; the debug variant additionally runs Delve on `:2345`.

## Plugin author's quick guide

Once you understand the boundary above, writing a plugin's Console
integration is mostly four steps. See
[Writing a plugin](writing-a-plugin) and [Custom UI](custom-ui) for the full
guides; this is the short version that ties Console integration together.

1. **Declare it in `definition.yaml`**. Map your custom HTML files to CRD
   kinds in `spec.customComponents`, and list the Kubernetes resources your
   UI may read in `spec.allowedResources` (group + version + resource +
   verbs). Anything not listed will return `forbidden` from the broker.
2. **Embed the assets**. Put your HTML/JS/CSS under `console/` in the plugin
   module, embed it with `//go:embed console`, and return it from
   `ConsoleAssets()`:

   ```go
   //go:embed console
   var consoleFS embed.FS

   func (p *Plugin) ConsoleAssets() http.FileSystem {
       return console.MustNewFileSystem(consoleFS)
   }
   ```

3. **Load the SDK from the host origin**. In each HTML page, read the
   `?host=` query parameter and inject `<script src="${host}/plugin-ui/plugin-sdk.js">`
   plus the matching stylesheet. Copy the `loadSdk()` helper from the
   cert-manager plugin's `_shared.js` as a starting point.
4. **Render**. `await fundament.init` to get context, call
   `fundament.k8s.list` / `.get` for data, and post
   `plugin:navigate` to follow a row into a detail view. See
   [Example: cert-manager](example-cert-manager) for a worked example.

## Verifying the integration end-to-end

When changing anything on the Console-plugin boundary, walk through the
full path in both modes:

1. **Mock mode** — `just dev` from the repo root. Open the Console, switch
   to a cluster that has the cert-manager plugin mock installed, and
   navigate to the Certificates list. In browser devtools:
   - Confirm the iframe loads (the asset comes from the kube-api-proxy with
     `Cache-Control: no-store`).
   - In the Console window, observe `plugin:ready` arriving from the iframe
     and the Console responding with `fundament:init`.
   - Click a row, confirm `plugin:navigate` followed by the detail view's
     `plugin:k8s:get` and a `fundament:k8s:result` with `ok: true`.
   - Edit `plugins/cert-manager/console/certificates-list.html`, refresh
     the browser, confirm the edit is live without rebuilding.

2. **Real mode** — `just dev -p local-gardener`. Install a real
   `PluginInstallation` against a shoot, wait for `status.phase = Running`,
   and repeat the trace above. Same protocol messages, but data now comes
   from the shoot's apiserver via the per-user SA token.

3. **Authorization spot-check** — temporarily remove an entry from a
   plugin's `allowedResources`, redeploy, and confirm the SDK call rejects
   with `SdkError('forbidden', ...)` and the host logs
   `[PluginIframe] rejected list request not in allowlist`.
