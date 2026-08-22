import { Routes } from '@angular/router';
import authGuard from './auth.guard';
import { tasksMatcher } from './tasks/task-views';
import { roundsMatcher } from './rounds/round-views';
import { catalogMatcher } from './catalog/catalog-views';
import { inventoryMatcher } from './inventory/inventory-views';
import { patchMappingMatcher } from './patch-mapping/patch-mapping-views';
import { dataCentersMatcher } from './datacenters/datacenter-views';
import { racksMatcher } from './racks/rack-views';

const routes: Routes = [
  {
    path: 'login',
    title: 'Log in',
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
        redirectTo: 'data-centers',
      },
      {
        // Before the detail route, not after: /catalog/all is one segment and
        // would otherwise be read as a product id. The matcher claims only the
        // shapes the menu makes, so an id still falls through to the page below.
        matcher: catalogMatcher,
        title: 'Catalog',
        loadComponent: () => import('./catalog/catalog').then((m) => m.default),
      },
      {
        path: 'catalog/:id',
        title: 'Product',
        loadComponent: () =>
          import('./catalog/catalog-detail/catalog-detail').then((m) => m.default),
      },
      {
        // Before the detail route, for the same reason as the catalog above.
        matcher: inventoryMatcher,
        title: 'Inventory',
        loadComponent: () => import('./inventory/inventory').then((m) => m.default),
      },
      {
        path: 'inventory/:id',
        title: 'Asset',
        loadComponent: () => import('./inventory/asset-detail/asset-detail').then((m) => m.default),
      },
      {
        // A data center is a place with an address of its own: /data-centers/ams1
        // is its floor map and /data-centers/ams1/layout the rooms and rows it is
        // built from. The short name is the slug, because that is what everybody
        // calls it.
        path: 'data-centers/:slug/layout',
        title: 'Layout',
        loadComponent: () =>
          import('./datacenters/datacenter-detail/datacenter-detail').then((m) => m.default),
      },
      {
        // The list and one data center on it are the same page, so they share
        // one route config: see the matcher.
        matcher: dataCentersMatcher,
        title: 'Data centers',
        loadComponent: () => import('./datacenters/datacenters').then((m) => m.default),
      },
      {
        path: 'racks/device/:id',
        title: 'Device',
        loadComponent: () => import('./racks/device-detail/device-detail').then((m) => m.default),
      },
      {
        // The list and one rack on it are the same page, so they share one
        // route config: see the matcher.
        matcher: racksMatcher,
        title: 'Racks',
        loadComponent: () => import('./racks/racks').then((m) => m.default),
      },
      {
        // A matcher instead of a path, like the sections above: every cable view
        // is one route, see patch-mapping-views.ts.
        matcher: patchMappingMatcher,
        title: 'Patch mapping',
        loadComponent: () => import('./patch-mapping/patch-mapping').then((m) => m.default),
      },
      {
        // A matcher instead of a path: every task view is one route, see
        // task-views.ts.
        matcher: tasksMatcher,
        title: 'Tasks',
        loadComponent: () => import('./tasks/tasks').then((m) => m.default),
      },
      {
        // The list of rounds and one round on it are the same page, so they
        // share one route config: see round-views.ts.
        matcher: roundsMatcher,
        title: 'Rounds',
        loadComponent: () => import('./rounds/rounds').then((m) => m.default),
      },
    ],
  },
  {
    path: 'task-management-technician',
    title: 'My rounds',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./task-management-technician/task-management-technician').then((m) => m.default),
  },
];
export default routes;
