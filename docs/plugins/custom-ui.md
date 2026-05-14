---
title: Custom UI
sidebar:
  order: 3
---

Plugins are responsible for shipping their own UI. For every CRD a plugin
exposes in the sidebar, the plugin must provide custom list and detail
views — the console does not generate fallback views from the CRD schema.

Plugin UIs run inside sandboxed iframes in the Fundament console. Each
plugin embeds its own HTML pages alongside the plugin binary and uses the
SDK provided by Fundament to talk to the host.

This page is the practical reference for plugin authors — what files to write and what the SDK gives you. For the architecture behind the iframe, including the full `postMessage` protocol and the kube-api-proxy mock/real modes, see [Console integration](console-integration).

## SDK files

The SDK is served by the Fundament console at a stable path under
`/plugin-ui/`. Because the iframe is sandboxed (`allow-scripts` only, no
`allow-same-origin`), it cannot statically link to the console origin —
the console passes its origin via a `?host=...` query parameter on the
iframe URL and your page injects the `<script>` and `<link>` tags using
that origin:

```js
const host = new URLSearchParams(location.search).get('host') ?? '';
const link = document.createElement('link');
link.rel = 'stylesheet';
link.href = `${host}/plugin-ui/plugin-sdk.css`;
document.head.appendChild(link);

await new Promise((resolve, reject) => {
  const script = document.createElement('script');
  script.src = `${host}/plugin-ui/plugin-sdk.js`;
  script.onload = resolve;
  script.onerror = reject;
  document.head.appendChild(script);
});
// window.fundament is now available.
```

The cert-manager plugin's `_shared.js:loadSdk()` is the reference
implementation — copy it as a starting point.

| File | Purpose |
|------|---------|
| `plugin-sdk.css` | Base styles, dark-mode support, and component classes |
| `plugin-sdk.js` | Sets `window.fundament`; handles the host↔plugin message protocol, theme application, iframe auto-resize, and the Kubernetes broker |

## SDK API

Once the SDK is loaded, use `window.fundament` instead of `postMessage`
directly. The SDK takes care of the protocol, request IDs, and timeouts.

```js
const ctx = await fundament.init;
// ctx: { theme, pluginName, crdKind, view, resource? }

const { items } = await fundament.k8s.list({
  group: 'cert-manager.io',
  version: 'v1',
  resource: 'certificates',
  namespace: ctx.resource?.namespace, // optional
});

const certificate = await fundament.k8s.get({
  group: 'cert-manager.io',
  version: 'v1',
  resource: 'certificates',
  name: ctx.resource.name,
  namespace: ctx.resource.namespace,
});

const unsubscribe = fundament.onThemeChange((theme) => { /* ... */ });
```

`init` resolves once with the initial context. `k8s.list` and `k8s.get` are
brokered by the console host — every call is validated against the
`allowedResources` declared in your `definition.yaml` and rejected with
`SdkError('forbidden', ...)` if not allowed. Each request times out after
10 seconds.

For navigating from a list row into a detail page, post `plugin:navigate`
yourself — the host resolves the path relative to the iframe's current
route:

```js
window.parent.postMessage(
  { type: 'plugin:navigate', name, namespace },
  '*',
);
```

The full message reference (including `plugin:ready`, `plugin:resize`, and
`fundament:k8s:result`) lives in
[Console integration](console-integration#the-postmessage-protocol).

## CSS component classes

`plugin-sdk.css` provides component classes built on Tailwind CSS. Dark mode is applied automatically by `plugin-sdk.js` via a `.dark` class on `<body>`.

| Class | Description |
|-------|-------------|
| `.plugin-card` | Bordered card container with rounded corners and padding |
| `.plugin-heading` | Primary heading (`h1`) |
| `.plugin-text` | Body / paragraph text |
| `.plugin-table` | Full-width data table with row dividers |

## Fetching data

The plugin iframe is sandboxed with `allow-scripts` only, runs with an
opaque origin, and cannot send the user's session cookie. Direct `fetch`
calls — to your own plugin backend or to the Kubernetes API — will not
carry credentials and should not be used.

Instead, read Kubernetes resources through `fundament.k8s.list` /
`fundament.k8s.get`. The console host validates every call against the
`allowedResources` declared in your `definition.yaml`, then forwards it to
the kube-api-proxy with the user's session. The plugin sees the result via
`postMessage`, never the user's credentials.

If you need a custom API beyond plain Kubernetes reads, expose it as a CRD
(or a subresource) and read it the same way — the broker is the only
supported data path for the iframe.

## Complete example

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>My plugin</title>
  </head>
  <body>
    <div class="plugin-card">
      <h1 class="plugin-heading" id="heading">Resources</h1>
      <table class="plugin-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Namespace</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody id="items">
          <tr><td colspan="3" class="plugin-text">Loading…</td></tr>
        </tbody>
      </table>
    </div>

    <script type="module">
      // Load the SDK from the console origin (passed in via ?host=...).
      const host = new URLSearchParams(location.search).get('host') ?? '';
      const link = document.createElement('link');
      link.rel = 'stylesheet';
      link.href = `${host}/plugin-ui/plugin-sdk.css`;
      document.head.appendChild(link);
      await new Promise((resolve, reject) => {
        const script = document.createElement('script');
        script.src = `${host}/plugin-ui/plugin-sdk.js`;
        script.onload = resolve;
        script.onerror = reject;
        document.head.appendChild(script);
      });

      const ctx = await fundament.init;
      document.getElementById('heading').textContent = ctx.crdKind;

      const { items } = await fundament.k8s.list({
        group: 'my-api.io',
        version: 'v1',
        resource: 'myresources',
      });

      const tbody = document.getElementById('items');
      tbody.innerHTML = '';
      for (const item of items) {
        const tr = document.createElement('tr');
        tr.innerHTML = `
          <td>${item.metadata.name}</td>
          <td>${item.metadata.namespace ?? ''}</td>
          <td>${item.status?.phase ?? '—'}</td>
        `;
        tbody.appendChild(tr);
      }
    </script>
  </body>
</html>
```

## Serving UI assets from Go

Implement `ConsoleProvider` in your plugin to serve the HTML pages:

```go
//go:embed console
var consoleFS embed.FS

type MyPlugin struct { ... }

func (p *MyPlugin) ConsoleAssets() http.FileSystem {
    return console.MustNewFileSystem(consoleFS)
}
```

The runtime mounts these assets at `/console/`. Your `definition.yaml` references them through `spec.customComponents`:

```yaml
spec:
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

`allowedResources` is the allowlist the console host checks every
`fundament.k8s.list` / `.get` call against — keep it in sync with what your
UI actually reads.

See [Writing a plugin](writing-a-plugin) for the full plugin setup and
[Console integration](console-integration) for the architecture behind the
iframe and the broker.
