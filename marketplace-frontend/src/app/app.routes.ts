import { Routes } from '@angular/router';

const routes: Routes = [
  // Public storefront
  {
    path: '',
    loadComponent: () => import('./marketplace/marketplace-home.component').then((m) => m.default),
  },
  {
    path: 'plugins/:name',
    loadComponent: () => import('./marketplace/plugin-detail.component').then((m) => m.default),
  },
  // Developer area
  // `create` is registered before `:name` so it is not parsed as a plugin name.
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
    path: 'manage/:name',
    loadComponent: () =>
      import('./plugin-development-detail/plugin-development-detail.component').then(
        (m) => m.default,
      ),
  },
  { path: '**', redirectTo: '' },
];

export default routes;
