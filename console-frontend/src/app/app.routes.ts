import { inject } from '@angular/core';
import { Router, type CanActivateFn, type Routes } from '@angular/router';
import authGuard from './auth.guard';
import clusterWizardGuard from './add-cluster-wizard-layout/cluster-wizard.guard';
import { OverlayService } from './overlay.service';

/**
 * Keeps an address for a sheet the shell owns. The sheet is not a page, so the
 * route has nothing to render: it opens the sheet and sends you home, and the
 * sheet appears over the home pane.
 */
const opensOverlay =
  (sheet: 'profile' | 'apiKeys' | 'newProject'): CanActivateFn =>
  () => {
    inject(OverlayService)[sheet].set(true);
    return inject(Router).parseUrl('/');
  };

const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./login/login.component').then((m) => m.default),
  },
  {
    path: '',
    canActivate: [authGuard],
    children: [
      {
        // Nothing in the main pane: the app opens on a choice, not on a list it
        // picked for you. A componentless route so the URL still matches while
        // the outlet stays empty.
        path: '',
        children: [],
      },
      {
        path: 'clusters',
        loadComponent: () => import('./dashboard/dashboard.component').then((m) => m.default),
        // The wizard is a child of the cluster list, so the list stays mounted
        // behind the sheet it opens in.
        children: [
          {
            path: 'add',
            loadComponent: () =>
              import('./add-cluster-wizard-layout/add-cluster-wizard-layout.component').then(
                (m) => m.default,
              ),
            children: [
              {
                path: '',
                loadComponent: () =>
                  import('./add-cluster/add-cluster.component').then((m) => m.default),
                canActivate: [clusterWizardGuard],
              },
              {
                path: 'nodes',
                loadComponent: () =>
                  import('./add-cluster-nodes/add-cluster-nodes.component').then((m) => m.default),
                canActivate: [clusterWizardGuard],
              },
              {
                path: 'summary',
                loadComponent: () =>
                  import('./add-cluster-summary/add-cluster-summary.component').then(
                    (m) => m.default,
                  ),
                canActivate: [clusterWizardGuard],
              },
              { path: '**', redirectTo: '' },
            ],
          },
        ],
      },
      {
        path: 'projects/add',
        canActivate: [opensOverlay('newProject')],
        children: [],
      },
      {
        path: 'profile',
        canActivate: [opensOverlay('profile')],
        children: [],
      },
      {
        path: 'api-keys',
        canActivate: [opensOverlay('apiKeys')],
        children: [],
      },
      {
        // Nothing in the main pane: the secondary sidebar is the project, and
        // you pick what to open from there. A componentless route so the URL
        // still matches while the outlet stays empty.
        path: 'projects/:id',
        children: [],
      },
      {
        path: 'projects/:id/general',
        loadComponent: () =>
          import('./project-detail/project-detail.component').then((m) => m.default),
      },
      {
        path: 'clusters/:id',
        loadComponent: () =>
          import('./cluster-details/cluster-details.component').then((m) => m.default),
        // Both editors open as a sheet over the detail page, so they are children
        // of it: the page behind the sheet stays mounted and keeps its scroll.
        children: [
          {
            path: 'nodes',
            loadComponent: () =>
              import('./cluster-nodes/cluster-nodes.component').then((m) => m.default),
          },
          {
            path: 'namespaces',
            loadComponent: () =>
              import('./cluster-namespaces/cluster-namespaces.component').then((m) => m.default),
          },
          {
            path: 'plugins',
            loadComponent: () =>
              import('./cluster-plugins/cluster-plugins.component').then((m) => m.default),
          },
        ],
      },
      {
        path: 'projects/:id/namespaces',
        loadComponent: () => import('./namespaces/namespaces.component').then((m) => m.default),
        children: [
          {
            // Everything about one namespace, the other way round from the member
            // sheet: who has access here. A route, so the row is a real link.
            path: ':name',
            loadComponent: () =>
              import('./namespace-sheet/namespace-sheet.component').then((m) => m.default),
          },
        ],
      },
      {
        path: 'projects/:id/members',
        loadComponent: () =>
          import('./project-members/project-members.component').then((m) => m.default),
        // The same reference as under organization members: a route of its own,
        // so the control that opens it can be a real link.
        children: [
          {
            path: 'permissions',
            loadComponent: () =>
              import('./permissions-sheet/permissions-sheet.component').then((m) => m.default),
          },
          {
            // Everything one member has here. A route, so the row that opens it
            // can be a real link.
            path: ':memberId',
            loadComponent: () =>
              import('./project-member-sheet/project-member-sheet.component').then(
                (m) => m.default,
              ),
          },
        ],
      },
      {
        path: 'projects/:id/limits',
        loadComponent: () =>
          import('./project-limits/project-limits.component').then((m) => m.default),
      },
      {
        path: 'projects/:id/settings',
        redirectTo: 'projects/:id',
        pathMatch: 'full',
      },
      {
        path: 'plugins',
        loadComponent: () => import('./plugins/plugins.component').then((m) => m.default),
      },
      {
        path: 'plugins/:id',
        loadComponent: () =>
          import('./plugin-details/plugin-details.component').then((m) => m.default),
      },
      {
        path: 'metrics',
        loadComponent: () => import('./metrics/metrics.component').then((m) => m.default),
      },
      {
        path: 'projects/:id/metrics',
        loadComponent: () => import('./metrics/metrics.component').then((m) => m.default),
      },
      {
        path: 'organization',
        loadComponent: () =>
          import('./organization-settings/organization-settings.component').then((m) => m.default),
      },
      {
        path: 'organization/members',
        loadComponent: () =>
          import('./organization-members/organization-members.component').then((m) => m.default),
        // A route of its own so the reference can be linked to, shared and
        // closed with the back button; a child so the list stays mounted behind
        // the sheet.
        children: [
          {
            path: 'permissions',
            loadComponent: () =>
              import('./permissions-sheet/permissions-sheet.component').then((m) => m.default),
          },
        ],
      },
      {
        path: 'organization/limits',
        loadComponent: () =>
          import('./organization-limits/organization-limits.component').then((m) => m.default),
      },
      // Plugin resource routes (organization-level)
      {
        path: 'plugin-resources/:pluginName',
        children: [
          {
            path: ':resourceKind',
            loadComponent: () =>
              import('./plugin-resources/resource-list/resource-list.component').then(
                (m) => m.default,
              ),
          },
          {
            // Must precede :resourceKind/:resourceId so `create` is not parsed as an id.
            path: ':resourceKind/create',
            loadComponent: () =>
              import('./plugin-resources/resource-create/resource-create.component').then(
                (m) => m.default,
              ),
          },
          {
            path: ':resourceKind/:resourceId',
            loadComponent: () =>
              import('./plugin-resources/resource-detail/resource-detail.component').then(
                (m) => m.default,
              ),
          },
        ],
      },
      // Plugin resource routes (project-level)
      {
        path: 'projects/:id/plugin-resources/:pluginName',
        children: [
          {
            path: ':resourceKind',
            loadComponent: () =>
              import('./plugin-resources/resource-list/resource-list.component').then(
                (m) => m.default,
              ),
          },
          {
            // Must precede :resourceKind/:resourceId so `create` is not parsed as an id.
            path: ':resourceKind/create',
            loadComponent: () =>
              import('./plugin-resources/resource-create/resource-create.component').then(
                (m) => m.default,
              ),
          },
          {
            path: ':resourceKind/:resourceId',
            loadComponent: () =>
              import('./plugin-resources/resource-detail/resource-detail.component').then(
                (m) => m.default,
              ),
          },
        ],
      },
    ],
  },
];

export default routes;
