# Fundament Marketplace frontend

A standalone Angular app for the Fundament plugin Marketplace. It has two areas:

- **Public storefront** (`/`) — browse and discover plugins, view plugin details.
- **Developer area** (`/manage`) — see and manage the plugins you author, track their
  review status, sideload builds, and learn how to publish a new plugin.

Styling uses [`@nldd/design-system`](https://www.npmjs.com/package/@nldd/design-system)
web components together with TailwindCSS v4 utility classes.

## Rendering

The app is a hybrid: the render mode is chosen per route in
[`src/app/app.routes.server.ts`](src/app/app.routes.server.ts).

- The **storefront** (`/`, `/plugins/:id`) is **server-rendered**. It is anonymous
  and its content is what search engines and link previews need to see, so each
  request is rendered by the Node server in [`src/server.ts`](src/server.ts) and
  hydrated in the browser.
- The **developer and admin areas** (`/manage`, `/admin`) are **client-rendered**.
  They sit behind a login, so a crawler has nothing to gain from them, and
  rendering them on the server would mean handing the visitor's session to the
  APIs and making every response user-specific. Angular serves the static shell
  and the browser takes over.

Things to keep in mind when working on server-rendered routes:

- **No browser globals at render time.** `window`, `localStorage`,
  `matchMedia` and friends do not exist under Node. Guard with
  `isPlatformBrowser` or run the code in `afterNextRender`.
  [`src/server-dom-shim.ts`](src/server-dom-shim.ts) fills in the one global a
  few design system components need at import time.
- **Nothing waits for your `fetch`.** The app is zoneless, so the renderer
  serializes as soon as it goes stable. API calls made through the Connect
  clients are covered — [`src/connect/transfer-cache.ts`](src/connect/transfer-cache.ts)
  registers a pending task and carries the response over to the browser so it is
  not fetched twice — but any other async work needs its own `PendingTasks` entry.
- **Web components upgrade on the client.** The server emits `<nldd-*>` tags
  with their light DOM; their shadow DOM only appears once the browser bundle
  runs. Keep page structure in the markup rather than depending on a component's
  internals for layout.
- **The theme is a cookie**, not localStorage, because the server has to read it
  to emit `<html class="dark">` in the first response. See
  [`src/app/theme.service.ts`](src/app/theme.service.ts).
- **No inline scripts.** The app is served with `script-src 'self'`, so anything
  inline is blocked. That is why the theme bootstrap is
  [`public/theme-init.js`](public/theme-init.js) rather than a `<script>` block,
  and why hydration is configured with `withNoIncrementalHydration()`: the event
  replay it would otherwise enable ships two inline scripts.

## Development

```sh
bun install
bun start        # dev server, server-rendering the same routes as production
bun run build    # production build (browser bundle + Node server)
bun run serve:ssr  # run the production server from dist/
bun run lint
bun run format
```

The server reads these environment variables:

| Variable                    | Purpose                                                          |
| --------------------------- | ---------------------------------------------------------------- |
| `PORT`                      | Listen port (default 4000)                                       |
| `CONNECT_SRC`               | Extra origins for the CSP `connect-src` directive                |
| `NG_ALLOWED_HOSTS`          | Comma-separated hostnames the renderer will answer for           |
| `MARKETPLACE_CONFIG_PATH`   | Where to read `config.json` (default: next to the browser build) |
| `CATALOG_API_INTERNAL_URL`  | In-cluster catalog API base URL, used only while rendering       |
| `REGISTRY_API_INTERNAL_URL` | In-cluster registry API base URL, used only while rendering      |
| `ADMIN_API_INTERNAL_URL`    | In-cluster admin API base URL, used only while rendering         |
