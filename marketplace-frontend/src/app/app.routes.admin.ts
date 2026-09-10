import { Routes } from '@angular/router';

// The review backoffice's route table, swapped in for app.routes.ts by the
// admin build configuration. Only the admin area ships in this build — and
// only this build is deployed on the restricted admin host, so the review UI
// never reaches a storefront visitor's browser (FUN-20).
const routes: Routes = [
  {
    path: 'admin/submissions/:id',
    loadComponent: () =>
      import('./admin-review/submission-detail.component').then((m) => m.default),
  },
  {
    path: 'admin',
    loadComponent: () => import('./admin-review/review-queue.component').then((m) => m.default),
  },
  { path: '', pathMatch: 'full', redirectTo: 'admin' },
  { path: '**', redirectTo: 'admin' },
];

export default routes;
