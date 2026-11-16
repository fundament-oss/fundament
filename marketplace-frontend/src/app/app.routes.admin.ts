import { Routes } from '@angular/router';
import staffAuthGuard from './admin-review/staff-auth.guard';

// The review backoffice's route table, swapped in for app.routes.ts by the
// admin build configuration. Only the admin area ships in this build — and
// only this build is deployed on the restricted admin host, so the review UI
// never reaches a storefront visitor's browser (FUN-20).
//
// Every review route is behind the staff login (see staff-auth.guard.ts). The
// guard sits on the `admin` parent, so a route added under it is covered
// without having to remember to opt in. canActivateChild rather than
// canActivate: it runs again on every navigation between the children.
const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./admin-review/login.component').then((m) => m.default),
  },
  {
    path: 'admin',
    canActivateChild: [staffAuthGuard],
    children: [
      {
        path: '',
        pathMatch: 'full',
        loadComponent: () => import('./admin-review/review-queue.component').then((m) => m.default),
      },
      {
        path: 'submissions/:id',
        loadComponent: () =>
          import('./admin-review/submission-detail.component').then((m) => m.default),
      },
    ],
  },
  { path: '', pathMatch: 'full', redirectTo: 'admin' },
  { path: '**', redirectTo: 'admin' },
];

export default routes;
