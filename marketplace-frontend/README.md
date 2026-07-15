# Fundament Marketplace frontend

A standalone Angular app for the Fundament plugin Marketplace. It has two areas:

- **Public storefront** (`/`) — browse and discover plugins, view plugin details.
- **Developer area** (`/manage`) — see and manage the plugins you author, track their
  review status, sideload builds, and learn how to publish a new plugin.

This app is currently a self-contained mockup: all data comes from in-memory mock
services (`marketplace.service.ts`, `plugin-development.service.ts`) and no calls are
made to a backend. The mock services intentionally mirror the shape of the future
`PluginService` API so they can be swapped for a real backend later.

Styling uses [`@nldd/design-system`](https://www.npmjs.com/package/@nldd/design-system)
web components together with TailwindCSS v4 utility classes.

## Development

```sh
bun install
bun start        # dev server
bun run build    # production build
bun run lint
bun run format
```
