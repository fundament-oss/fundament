import { Routes } from '@angular/router';
import authGuard from './auth.guard';

// The developer portal's route table, swapped in for app.routes.ts by the
// registry build configuration. Only the publishing area ships in this build:
// the storefront and the backoffice are their own deployables (FUN-20), linked
// by URL rather than by route.
//
// Every publishing route is behind the console's login (see auth.guard.ts).
// The guard sits on the `manage` parent, so a route added under it is covered
// without having to remember to opt in. canActivateChild rather than
// canActivate: it runs again on every navigation between the children, where
// a parent's canActivate would only run on entering `manage`.
const routes: Routes = [
  {
    path: 'manage',
    canActivateChild: [authGuard],
    children: [
      {
        path: '',
        pathMatch: 'full',
        loadComponent: () =>
          import('./plugin-development/plugin-development.component').then((m) => m.default),
      },
      // `create` is registered before `:id` so it is not parsed as a plugin id.
      {
        path: 'create',
        loadComponent: () =>
          import('./plugin-create/plugin-create.component').then((m) => m.default),
      },
      {
        path: ':id',
        loadComponent: () =>
          import('./plugin-development-detail/plugin-development-detail.component').then(
            (m) => m.default,
          ),
      },
    ],
  },
  { path: '', pathMatch: 'full', redirectTo: 'manage' },
  { path: '**', redirectTo: 'manage' },
];

export default routes;
