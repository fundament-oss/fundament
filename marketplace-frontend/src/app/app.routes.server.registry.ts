import { RenderMode, ServerRoute } from '@angular/ssr';

// The developer portal is behind a login, so every route is client-rendered:
// there is nothing to offer a crawler, and rendering on the server would mean
// forwarding the visitor's session to the APIs. Swapped in for
// app.routes.server.ts by the registry build configuration.
const serverRoutes: ServerRoute[] = [{ path: '**', renderMode: RenderMode.Client }];

export default serverRoutes;
