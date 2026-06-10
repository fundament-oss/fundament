import { Routes } from '@angular/router';
import authGuard from './auth.guard';

const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./login/login').then((m) => m.default),
  },
  {
    path: '',
    canActivate: [authGuard],
    loadComponent: () => import('./shell/shell').then((m) => m.default),
    children: [
      {
        path: '',
        pathMatch: 'full',
        redirectTo: 'datacenters',
      },
      {
        path: 'catalog/:id',
        loadComponent: () =>
          import('./catalog/catalog-detail/catalog-detail').then((m) => m.default),
      },
      {
        path: 'catalog',
        loadComponent: () => import('./catalog/catalog').then((m) => m.default),
      },
      {
        path: 'inventory/:id',
        loadComponent: () => import('./inventory/asset-detail/asset-detail').then((m) => m.default),
      },
      {
        path: 'inventory',
        loadComponent: () => import('./inventory/inventory').then((m) => m.default),
      },
      {
        path: 'datacenters/:id',
        loadComponent: () =>
          import('./datacenters/datacenter-detail/datacenter-detail').then((m) => m.default),
      },
      {
        path: 'datacenters',
        loadComponent: () => import('./datacenters/datacenters').then((m) => m.default),
      },
      {
        path: 'racks/device/:id',
        loadComponent: () => import('./racks/device-detail/device-detail').then((m) => m.default),
      },
      {
        path: 'racks/:rackId',
        loadComponent: () => import('./racks/racks').then((m) => m.default),
      },
      {
        path: 'racks',
        loadComponent: () => import('./racks/racks').then((m) => m.default),
      },
      {
        path: 'patch-mapping',
        loadComponent: () => import('./patch-mapping/patch-mapping').then((m) => m.default),
      },
      {
        path: 'task-management-admin',
        loadComponent: () =>
          import('./task-management-admin/task-management-admin').then((m) => m.default),
      },
    ],
  },
  {
    path: 'task-management-technician',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./task-management-technician/task-management-technician').then((m) => m.default),
  },
];
export default routes;
