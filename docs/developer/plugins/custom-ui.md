---
title: Custom UI
sidebar:
  order: 5
---

Custom UI is optional. When a plugin omits `customComponents` for a CRD it exposes in the sidebar, the console renders a generated list and detail view from the CRD's OpenAPI schema (see [Console integration](console-integration#generated-fallback-ui)). Ship a custom UI when you need create or edit actions or a bespoke layout: this page covers that case.

Plugin UIs run inside sandboxed iframes on the plugin-proxy origin. Each plugin embeds its own HTML pages in the plugin binary and uses the SDK to talk to the console and the cluster.

This page is the practical reference for plugin authors: what files to write and what the SDK gives you. For the architecture behind the iframe, the full `postMessage` protocol and the kube-api-proxy modes, see [Console integration](console-integration).

## Page layout

```
console/
├── _shared.js               loadSdk() and helpers
├── <plural>-list.html       customComponents.<Kind>.list
├── <plural>-list.js
├── <plural>-detail.html     customComponents.<Kind>.detail
├── <plural>-detail.js
├── <plural>-create.html     customComponents.<Kind>.create
└── <plural>-create.js
```

- Each HTML page loads its logic from a `.js` module: the page CSP blocks inline scripts and styles.
- Working example: [`plugins/cert-manager/console/`](https://github.com/fundament-oss/fundament/tree/master/plugins/cert-manager/console).

## SDK files

plugin-proxy serves the SDK on the page's own origin under `/plugins/sdk/v1/`:

| File | Purpose |
|------|---------|
| `plugin-sdk.css` | Base styles, light/dark theme and component classes |
| `plugin-sdk.js` | Sets `window.fundament`; handles the console message protocol, theme, iframe resize, the PluginToken and Kubernetes calls |
| `nldd-design-system.js` | NLDD Design System web components |

The cert-manager plugin's `_shared.js:loadSdk()` is the reference implementation: copy it as a starting point.

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

## SDK API

Once the SDK is loaded, use `window.fundament`. The SDK takes care of the protocol, the PluginToken and timeouts.

```js
const ctx = await fundament.init;
// ctx: { theme, pluginName, crdKind, view, resource?, namespaces?, kubeApiProxyUrl, clusterId }

const res = { group: 'my-api.io', version: 'v1', resource: 'myresources' };

const { items } = await fundament.k8s.list({ ...res, namespace: ctx.resource?.namespace });
const item = await fundament.k8s.get({ ...res, name: ctx.resource.name, namespace: ctx.resource.namespace });

await fundament.k8s.create({ ...res, namespace: 'team-a' }, {
  apiVersion: 'my-api.io/v1',
  kind: 'MyResource',
  metadata: { name: 'example', namespace: 'team-a' },
  spec: {},
});
await fundament.k8s.patch({ ...res, name: 'example', namespace: 'team-a' }, { spec: { size: 2 } }); // merge patch
await fundament.k8s.delete({ ...res, name: 'example', namespace: 'team-a' });

const unsubscribe = fundament.onThemeChange((theme) => { /* ... */ });
```

| Member | Use |
|---|---|
| `init` | Resolves once with the context |
| `k8s.list`, `k8s.get`, `k8s.create`, `k8s.patch`, `k8s.delete` | Kubernetes API calls through kube-api-proxy |
| `fetch`, `getToken` | Other calls to kube-api-proxy or plugin-proxy, with the PluginToken |
| `parentOrigin` | The console origin, for messages to the console |
| `onThemeChange` | Callback on a console theme change; returns an unsubscribe function |

Calls reject with an `SdkError`: `timeout` after 30 s, `unauthorized` when a refreshed token is rejected, `forbidden` on 403, `http` with the Kubernetes `Status` message for other errors, `transport` on network errors.

For navigating from a list row into a detail page, post `plugin:navigate`; the console resolves the route relative to the current one:

```js
export function navigateToDetail(name, namespace) {
  window.parent.postMessage(
    { type: 'plugin:navigate', name, namespace },
    window.fundament?.parentOrigin ?? '*',
  );
}
```

The full message reference lives in [Console integration](console-integration#the-postmessage-protocol).

## CSS component classes

`plugin-sdk.css` provides component classes built on Tailwind CSS. `plugin-sdk.js` applies the console theme as a `light` or `dark` class on `<body>`.

| Class | Description |
|-------|-------------|
| `.plugin-card` | Bordered card container with rounded corners and padding |
| `.plugin-heading` | Primary heading (`h1`) |
| `.plugin-text` | Body / paragraph text |
| `.plugin-table` | Full-width data table with row dividers |

Forms and other elements: `.plugin-actions`, `.plugin-button`, `.plugin-button-secondary`, `.plugin-checkbox`, `.plugin-deflist`, `.plugin-error`, `.plugin-field`, `.plugin-fieldset`, `.plugin-form`, `.plugin-hint`, `.plugin-icon`, `.plugin-input`, `.plugin-label`, `.plugin-legend`, `.plugin-row`, `.plugin-select`.

## Fetching data

- `fundament.k8s.*` calls `https://<kube-api-proxy>/clusters/<cluster-id>/api…` directly from the page, with the PluginToken the console sent in `fundament:init`.
- A call succeeds when both the user and the plugin's ServiceAccount may make it. The plugin's part is `permissions.rbac` in your `definition.yaml`: grant what your pages read and write. Steps: [Console integration](console-integration#kubernetes-call-path).
- For other calls to kube-api-proxy or plugin-proxy use `fundament.fetch`: it attaches the PluginToken and refreshes it on 401.
- In the local sandbox the plugin's part does not restrict page calls yet: see the TODO under [Kubernetes call path](console-integration#kubernetes-call-path).

## Content Security Policy

plugin-proxy sends this header with every page:

```text
default-src 'self'; script-src 'self'; style-src 'self';
connect-src <kube-api-proxy> <plugin-proxy>; form-action <kube-api-proxy> <plugin-proxy>;
frame-ancestors <console>; base-uri 'none'; object-src 'none'
```

- No inline `<script>`, `<style>` or `style=""`.
- Network calls go to kube-api-proxy and plugin-proxy only.

## Complete example

`console/myresources-list.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>MyResources</title>
  </head>
  <body>
    <div class="plugin-card">
      <h1 class="plugin-heading">MyResources</h1>
      <table class="plugin-table">
        <thead>
          <tr><th>Name</th><th>Namespace</th></tr>
        </thead>
        <tbody id="rows"></tbody>
      </table>
    </div>
    <script type="module" src="./myresources-list.js"></script>
  </body>
</html>
```

`console/myresources-list.js`:

```js
import { loadSdk, navigateToDetail } from './_shared.js';

await loadSdk();
await fundament.init;

const { items } = await fundament.k8s.list({
  group: 'my-api.io',
  version: 'v1',
  resource: 'myresources',
});

const tbody = document.getElementById('rows');
for (const item of items ?? []) {
  const row = tbody.insertRow();
  const link = document.createElement('a');
  link.href = '#';
  link.textContent = item.metadata.name;
  link.addEventListener('click', (e) => {
    e.preventDefault();
    navigateToDetail(item.metadata.name, item.metadata.namespace);
  });
  row.insertCell().append(link);
  row.insertCell().textContent = item.metadata.namespace ?? '';
}
```

## Serving UI assets from Go

Implement `ConsoleProvider` in your plugin to serve the pages:

```go
//go:embed console/*
var consoleFiles embed.FS

func (p *MyPluginPlugin) ConsoleAssets() http.FileSystem {
    return console.NewFileSystem(consoleFiles, "console")
}
```

The runtime mounts these assets at `/console/`. Your `definition.yaml` references them through `spec.customComponents` and grants the pages' access in `spec.permissions.rbac`:

```yaml
spec:
  permissions:
    rbac:
      - apiGroups: ["my-api.io"]
        resources: ["myresources"]
        verbs: ["get", "list", "watch"]

  customComponents:
    MyResource:
      list: myresources-list.html       # served from /console/myresources-list.html
      detail: myresources-detail.html   # served from /console/myresources-detail.html

  allowedResources:
    - group: my-api.io
      version: v1
      resource: myresources
      verbs: [get, list]
```

`allowedResources` is not checked yet: see the TODO under [Discovery and registration](console-integration#discovery-and-registration).

See [Writing a plugin](writing-a-plugin) for the full plugin setup.
