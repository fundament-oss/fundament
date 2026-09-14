import { RenderMode, ServerRoute } from '@angular/ssr';

/**
 * Render mode for the public storefront (the default build): server-rendered
 * throughout. It is anonymous, its content is the thing search engines and
 * link previews need to see, and the catalog call it depends on is cheap.
 *
 * The developer portal and the review backoffice are separate builds with
 * their own server-route tables (app.routes.server.registry.ts /
 * app.routes.server.admin.ts), client-rendered because they sit behind a
 * login.
 */
const serverRoutes: ServerRoute[] = [
  { path: '', renderMode: RenderMode.Server },
  { path: 'plugins/:id', renderMode: RenderMode.Server },
  // Unknown paths redirect to the storefront (see app.routes.ts).
  { path: '**', renderMode: RenderMode.Server },
];

export default serverRoutes;
