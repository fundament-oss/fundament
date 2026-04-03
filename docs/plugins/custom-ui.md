---
title: Custom UI
sidebar:
  order: 3
---

When a plugin does not provide a custom UI, the Fundament console automatically generates read-only list and detail views for each CRD the plugin manages. These default views are derived directly from the CRD schema: the list view uses `additionalPrinterColumns` to build its table columns, and the detail view renders the resource's spec and status fields. No extra configuration is needed to get this default UI.

Custom UIs replace the default views for the CRDs you specify. Plugin UIs run inside sandboxed iframes in the Fundament console. Each plugin serves its own HTML pages from its Go backend and includes two SDK files provided by Fundament.

## SDK files

Every plugin HTML page must include both SDK files. They are served by the Fundament console and are available at a stable, versioned path:

```html
<link rel="stylesheet" href="/plugin-ui/plugin-sdk.css" />
<script src="/plugin-ui/plugin-sdk.js"></script>
```

| File | Purpose |
|------|---------|
| `plugin-sdk.css` | Base styles, dark-mode support, and component classes |
| `plugin-sdk.js` | Handles the host↔plugin message protocol, theme application, and iframe auto-resize |

## Message protocol

The host sends messages to the plugin after it signals readiness.

### Plugin → host (sent automatically by `plugin-sdk.js`)

| Message | When | Payload |
|---------|------|---------|
| `plugin:ready` | On script load | _(no payload)_ |
| `plugin:resize` | When content height changes | `{ height: number }` |
| `plugin:navigate` | When the plugin navigates | `{ path: string }` |

### Host → plugin

| Message | When | Payload |
|---------|------|---------|
| `fundament:init` | After `plugin:ready` | `{ theme, pluginName, crdKind, view }` |
| `fundament:theme-changed` | User switches theme | `{ theme: 'light' \| 'dark' }` |

The `fundament:init` message contains everything the plugin needs to render the correct view:

| Field | Description |
|-------|-------------|
| `theme` | `'light'` or `'dark'` — applied automatically by the SDK |
| `pluginName` | Name of the installed plugin |
| `crdKind` | The CRD kind being shown (e.g. `certificates.cert-manager.io`) |
| `view` | `'list'` or `'detail'` |

## CSS component classes

`plugin-sdk.css` provides component classes built on Tailwind CSS. Dark mode is applied automatically by `plugin-sdk.js` via a `.dark` class on `<body>`.

| Class | Description |
|-------|-------------|
| `.plugin-card` | Bordered card container with rounded corners and padding |
| `.plugin-heading` | Primary heading (`h1`) |
| `.plugin-text` | Body / paragraph text |
| `.plugin-table` | Full-width data table with row dividers |

## Fetching data

The plugin page is served from the plugin's own Go backend (via `ConsoleProvider`). Use `fetch` with relative URLs to call your backend's API:

```js
const response = await fetch('/api/resources');
const data = await response.json();
```

Your Go backend handles authentication — the Fundament console proxies requests to your plugin's service, so session credentials are not exposed to the iframe.

## Complete example

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>My plugin</title>
    <link rel="stylesheet" href="/plugin-ui/plugin-sdk.css" />
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

    <script src="/plugin-ui/plugin-sdk.js"></script>
    <script>
      window.addEventListener('message', async (event) => {
        const data = event.data;
        if (data?.type !== 'fundament:init') return;

        // Update the heading to show the resource kind from context.
        document.getElementById('heading').textContent = data.crdKind;

        // Fetch resources from the plugin backend.
        const response = await fetch('/api/resources');
        const items = await response.json();

        const tbody = document.getElementById('items');
        tbody.innerHTML = '';
        for (const item of items) {
          const tr = document.createElement('tr');
          tr.innerHTML = `
            <td>${item.metadata.name}</td>
            <td>${item.metadata.namespace}</td>
            <td>${item.status?.phase ?? '—'}</td>
          `;
          tbody.appendChild(tr);
        }
      });
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

The runtime mounts these assets at `/console/`. Your `definition.yaml` references them:

```yaml
menu:
  project:
    - crd: myresources.my-api.io
      list: true    # served from /console/list.html
      detail: true  # served from /console/detail.html
```

See [Writing a plugin](writing-a-plugin) for the full plugin setup.
