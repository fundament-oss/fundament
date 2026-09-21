import { Routes } from '@angular/router';
import authGuard from './auth.guard';

// The developer portal's route table, swapped in for app.routes.ts by the
// registry build configuration. Only the publishing area ships in this build:
// the storefront and the backoffice are their own deployables (FUN-20), linked
// by URL rather than by route.
//
// Every publishing route is behind the console's login (see auth.guard.ts).
// The guard is repeated per route rather than hoisted onto a parent `manage`
// route, which would have to carry the redirects below as children and give
// the demo bundle's `startsWith('manage')` filter a different shape to match.
const routes: Routes = [
  // `create` is registered before `:id` so it is not parsed as a plugin id.
  {
    path: 'manage/create',
    canActivate: [authGuard],
    loadComponent: () => import('./plugin-create/plugin-create.component').then((m) => m.default),
  },
  {
    path: 'manage',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./plugin-development/plugin-development.component').then((m) => m.default),
  },
  {
    path: 'manage/:id',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./plugin-development-detail/plugin-development-detail.component').then(
        (m) => m.default,
      ),
  },
  { path: '', pathMatch: 'full', redirectTo: 'manage' },
  { path: '**', redirectTo: 'manage' },
];

export default routes;
