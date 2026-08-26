import { RenderMode, ServerRoute } from '@angular/ssr';

/**
 * Render mode per area.
 *
 * The public storefront is server-rendered: it is anonymous, its content is the
 * thing search engines and link previews need to see, and the catalog call it
 * depends on is cheap.
 *
 * The developer and backoffice areas are client-rendered. They are behind a
 * login, so they have nothing to offer a crawler, and rendering them on the
 * server would mean forwarding the visitor's session to the APIs and making
 * every response uncacheable and user-specific. Angular serves the static CSR
 * shell for these routes and the browser takes it from there.
 */
const serverRoutes: ServerRoute[] = [
  { path: '', renderMode: RenderMode.Server },
  { path: 'plugins/:id', renderMode: RenderMode.Server },
  { path: 'manage', renderMode: RenderMode.Client },
  { path: 'manage/**', renderMode: RenderMode.Client },
  { path: 'admin', renderMode: RenderMode.Client },
  { path: 'admin/**', renderMode: RenderMode.Client },
  // Unknown paths redirect to the storefront (see app.routes.ts).
  { path: '**', renderMode: RenderMode.Server },
];

export default serverRoutes;
