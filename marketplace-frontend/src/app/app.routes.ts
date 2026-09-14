import { Routes } from '@angular/router';

// The public storefront's route table — the default build. The developer
// portal and the review backoffice are their own deployables over the same
// source (FUN-20): their build configurations swap this file for
// app.routes.registry.ts / app.routes.admin.ts, so neither area ships to a
// storefront visitor's browser.
const routes: Routes = [
  {
    path: '',
    loadComponent: () => import('./marketplace/index.component').then((m) => m.default),
  },
  {
    path: 'plugins/:id',
    loadComponent: () => import('./marketplace/plugin-detail.component').then((m) => m.default),
  },
  { path: '**', redirectTo: '' },
];

export default routes;
