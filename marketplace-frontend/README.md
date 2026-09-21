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

## Signing in

The developer portal has no session of its own. The console's `fundament_auth`
cookie is set by `authn-api` on the parent domain (`COOKIE_DOMAIN`), so the
browser sends it to the portal's APIs too, and `marketplace-registry-api`
validates exactly that cookie, issuer and audience.

So signing in is a hand-off: [`src/app/auth.guard.ts`](src/app/auth.guard.ts)
asks `authn.v1 GetUserInfo` whether there is a session, and if there is not it
sends the browser to a login that sets that cookie.
[`SessionService.loginUrl`](src/app/session.service.ts) picks which:

- **The console's own login page**, `consoleUrl/login?app=marketplace-registry&path=<this
route>`, wherever a console is deployed. There is one password form in the
  product and the console owns it, so there is one place to later add rate
  limiting, lockout or a password reset. The console is handed an _app name_
  rather than a URL, and resolves where to send the visitor back to against its
  own configuration — so it needs no allowlist of its own and cannot be turned
  into an open redirect. See its
  [`login/login-handoff.ts`](../console-frontend/src/app/login/login-handoff.ts),
  and give it `developerUrl` in its `config.json`.
- **`authnApiUrl/login?return_to=<this route>`** otherwise, which is dex's own
  page. `authn-api` runs the OIDC round trip, sets the cookie and redirects
  back. It only honours a `return_to` whose origin it serves, so the portal's
  own origin has to be in authn's `CORS_ALLOWED_ORIGINS` — which it needs
  regardless, to call `GetUserInfo` with the cookie attached.

Password login is an OAuth password grant, which only works while dex
authenticates against its own store (`passwordConnector: local`). The day
Fundament federates to a real identity provider, the console's form stops
working and the redirect becomes the only path, for the console as much as for
the portal.

The hand-off is attempted once per tab. If the browser comes back from the
login and `GetUserInfo` still says there is no session, the guard lets the
navigation through and leaves the API's own error to surface, rather than
sending the visitor round again: `GetUserInfo` failing does not distinguish
"not signed in" from "authn did not answer", and a second pass would loop the
tab silently, because dex re-approves the session it just minted every time.

Configuration that has to agree for this to work: `authnApiUrl` in the
deployment's `config.json` (a build without it has no session surface, which is
how the storefront and the demo bundle opt out), `consoleUrl` here and
`developerUrl` on the console for the hand-off, and the portal's origin on
authn's CORS list. The review backoffice is deliberately not part of this:
reviewers are Fundament staff and authenticate against `dcim-authn-api`
(FUN-20).

## Development

```sh
bun install
bun start        # dev server, server-rendering the same routes as production
bun run build    # production build (browser bundle + Node server)

# One source, three deployables (FUN-20): the storefront is the default build;
# the developer portal and the review backoffice swap the route tables via
# fileReplacements and deploy on their own hosts.
bun run start:registry   # developer portal dev server
bun run start:admin      # review backoffice dev server
bun run build:registry
bun run build:admin
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
