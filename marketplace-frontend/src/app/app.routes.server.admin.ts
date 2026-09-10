import { RenderMode, ServerRoute } from '@angular/ssr';

// The backoffice is behind a login, so every route is client-rendered: there
// is nothing to offer a crawler, and rendering on the server would mean
// forwarding the reviewer's session to the API. Swapped in for
// app.routes.server.ts by the admin build configuration.
const serverRoutes: ServerRoute[] = [{ path: '**', renderMode: RenderMode.Client }];

export default serverRoutes;
