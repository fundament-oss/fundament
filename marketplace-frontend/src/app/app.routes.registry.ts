import { Routes } from '@angular/router';

// The developer portal's route table, swapped in for app.routes.ts by the
// registry build configuration. Only the publishing area ships in this build:
// the storefront and the backoffice are their own deployables (FUN-20), linked
// by URL rather than by route.
const routes: Routes = [
  // `create` is registered before `:id` so it is not parsed as a plugin id.
  {
    path: 'manage/create',
    loadComponent: () => import('./plugin-create/plugin-create.component').then((m) => m.default),
  },
  {
    path: 'manage',
    loadComponent: () =>
      import('./plugin-development/plugin-development.component').then((m) => m.default),
  },
  {
    path: 'manage/:id',
    loadComponent: () =>
      import('./plugin-development-detail/plugin-development-detail.component').then(
        (m) => m.default,
      ),
  },
  { path: '', pathMatch: 'full', redirectTo: 'manage' },
  { path: '**', redirectTo: 'manage' },
];

export default routes;
