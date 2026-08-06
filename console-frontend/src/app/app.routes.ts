import { Routes } from '@angular/router';
import authGuard from './auth.guard';
import clusterWizardGuard from './add-cluster-wizard-layout/cluster-wizard.guard';

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
        path: '',
        loadComponent: () => import('./dashboard/dashboard.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Clusters' }],
        },
        // The wizard is a child of the cluster list, so the list stays mounted
        // behind the sheet it opens in.
        children: [
          {
            path: 'clusters/add',
            loadComponent: () =>
              import('./add-cluster-wizard-layout/add-cluster-wizard-layout.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [
                { label: 'Clusters', route: '/' },
                { label: 'New cluster', route: '/clusters/add' },
              ],
            },
            children: [
              {
                path: '',
                loadComponent: () =>
                  import('./add-cluster/add-cluster.component').then((m) => m.default),
                canActivate: [clusterWizardGuard],
                data: {
                  breadcrumbs: [{ label: 'Basics' }],
                },
              },
              {
                path: 'nodes',
                loadComponent: () =>
                  import('./add-cluster-nodes/add-cluster-nodes.component').then((m) => m.default),
                canActivate: [clusterWizardGuard],
                data: {
                  breadcrumbs: [{ label: 'Node pools' }],
                },
              },
              {
                path: 'summary',
                loadComponent: () =>
                  import('./add-cluster-summary/add-cluster-summary.component').then(
                    (m) => m.default,
                  ),
                canActivate: [clusterWizardGuard],
                data: {
                  breadcrumbs: [{ label: 'Summary' }],
                },
              },
              { path: '**', redirectTo: '' },
            ],
          },
        ],
      },
      {
        // A sheet with nothing behind it: the projects live in the sidebar now,
        // so there is no list page for it to open over.
        path: 'projects/add',
        loadComponent: () => import('./add-project/add-project.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'New project' }],
        },
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
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'General' }],
        },
      },
      {
        path: 'clusters/:id',
        loadComponent: () =>
          import('./cluster-details/cluster-details.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Clusters', route: '/' }, { label: ':clusterName' }],
        },
        // Both editors open as a sheet over the detail page, so they are children
        // of it: the page behind the sheet stays mounted and keeps its scroll.
        children: [
          {
            path: 'nodes',
            loadComponent: () =>
              import('./cluster-nodes/cluster-nodes.component').then((m) => m.default),
            data: {
              breadcrumbs: [
                { label: 'Clusters', route: '/' },
                { label: ':clusterName', route: '/clusters/:id' },
                { label: 'Nodes' },
              ],
            },
          },
          {
            path: 'namespaces',
            loadComponent: () =>
              import('./cluster-namespaces/cluster-namespaces.component').then((m) => m.default),
            data: {
              breadcrumbs: [
                { label: 'Clusters', route: '/' },
                { label: ':clusterName', route: '/clusters/:id' },
                { label: 'Namespaces' },
              ],
            },
          },
          {
            path: 'plugins',
            loadComponent: () =>
              import('./cluster-plugins/cluster-plugins.component').then((m) => m.default),
            data: {
              breadcrumbs: [
                { label: 'Clusters', route: '/' },
                { label: ':clusterName', route: '/clusters/:id' },
                { label: 'Plugins' },
              ],
            },
          },
        ],
      },
      {
        path: 'projects/:id/namespaces',
        loadComponent: () => import('./namespaces/namespaces.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Namespaces' }],
        },
        children: [
          {
            // Everything about one namespace, the other way round from the member
            // sheet: who has access here. A route, so the row is a real link.
            path: ':name',
            loadComponent: () =>
              import('./namespace-sheet/namespace-sheet.component').then((m) => m.default),
            data: {
              breadcrumbs: [
                { label: ':projectName', route: '/projects/:id' },
                { label: 'Namespaces', route: '/projects/:id/namespaces' },
                { label: 'Namespace' },
              ],
            },
          },
        ],
      },
      {
        path: 'projects/:id/members',
        loadComponent: () =>
          import('./project-members/project-members.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Members' }],
        },
        // The same reference as under organization members: a route of its own,
        // so the control that opens it can be a real link.
        children: [
          {
            path: 'permissions',
            loadComponent: () =>
              import('./permissions-sheet/permissions-sheet.component').then((m) => m.default),
            data: {
              breadcrumbs: [
                { label: ':projectName', route: '/projects/:id' },
                { label: 'Members', route: '/projects/:id/members' },
                { label: 'About permissions' },
              ],
            },
          },
          {
            // Everything one member has here. A route, so the row that opens it
            // can be a real link.
            path: ':memberId',
            loadComponent: () =>
              import('./project-member-sheet/project-member-sheet.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [
                { label: ':projectName', route: '/projects/:id' },
                { label: 'Members', route: '/projects/:id/members' },
                { label: 'Member' },
              ],
            },
          },
        ],
      },
      {
        path: 'projects/:id/limits',
        loadComponent: () =>
          import('./project-limits/project-limits.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Limits' }],
        },
      },
      {
        path: 'projects/:id/settings',
        redirectTo: 'projects/:id',
        pathMatch: 'full',
      },
      {
        path: 'plugins',
        loadComponent: () => import('./plugins/plugins.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Plugins' }],
        },
      },
      {
        path: 'profile',
        loadComponent: () => import('./profile/profile.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Profile' }],
        },
      },
      {
        path: 'plugins/:id',
        loadComponent: () =>
          import('./plugin-details/plugin-details.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Plugins', route: '/plugins' }, { label: 'Plugin details' }],
        },
      },
      {
        path: 'metrics',
        loadComponent: () => import('./metrics/metrics.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Metrics' }],
        },
      },
      {
        path: 'projects/:id/metrics',
        loadComponent: () => import('./metrics/metrics.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Metrics' }],
        },
      },
      {
        path: 'organization',
        loadComponent: () =>
          import('./organization-settings/organization-settings.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Settings' }],
        },
      },
      {
        path: 'organization/members',
        loadComponent: () =>
          import('./organization-members/organization-members.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Organization members' }],
        },
        // A route of its own so the reference can be linked to, shared and
        // closed with the back button; a child so the list stays mounted behind
        // the sheet.
        children: [
          {
            path: 'permissions',
            loadComponent: () =>
              import('./permissions-sheet/permissions-sheet.component').then((m) => m.default),
            data: {
              breadcrumbs: [
                { label: 'Organization members', route: '/organization/members' },
                { label: 'About permissions' },
              ],
            },
          },
        ],
      },
      {
        path: 'organization/limits',
        loadComponent: () =>
          import('./organization-limits/organization-limits.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Limits' }],
        },
      },
      {
        path: 'api-keys',
        loadComponent: () => import('./api-keys/api-keys.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'API keys' }],
        },
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
            data: {
              breadcrumbs: [{ label: ':pluginAlias' }, { label: ':resourceKindLabel' }],
            },
          },
          {
            // Must precede :resourceKind/:resourceId so `create` is not parsed as an id.
            path: ':resourceKind/create',
            loadComponent: () =>
              import('./plugin-resources/resource-create/resource-create.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [
                { label: ':pluginAlias' },
                {
                  label: ':resourceKindLabel',
                  route: '/plugin-resources/:pluginName/:resourceKind',
                },
                { label: 'Create' },
              ],
            },
          },
          {
            path: ':resourceKind/:resourceId',
            loadComponent: () =>
              import('./plugin-resources/resource-detail/resource-detail.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [
                { label: ':pluginAlias' },
                {
                  label: ':resourceKindLabel',
                  route: '/plugin-resources/:pluginName/:resourceKind',
                },
                { label: ':resourceName' },
              ],
            },
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
            data: {
              breadcrumbs: [
                { label: ':projectName', route: '/projects/:id' },
                { label: ':pluginAlias' },
                { label: ':resourceKindLabel' },
              ],
            },
          },
          {
            // Must precede :resourceKind/:resourceId so `create` is not parsed as an id.
            path: ':resourceKind/create',
            loadComponent: () =>
              import('./plugin-resources/resource-create/resource-create.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [
                { label: ':projectName', route: '/projects/:id' },
                { label: ':pluginAlias' },
                {
                  label: ':resourceKindLabel',
                  route: '/projects/:id/plugin-resources/:pluginName/:resourceKind',
                },
                { label: 'Create' },
              ],
            },
          },
          {
            path: ':resourceKind/:resourceId',
            loadComponent: () =>
              import('./plugin-resources/resource-detail/resource-detail.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [
                { label: ':projectName', route: '/projects/:id' },
                { label: ':pluginAlias' },
                {
                  label: ':resourceKindLabel',
                  route: '/projects/:id/plugin-resources/:pluginName/:resourceKind',
                },
                { label: ':resourceName' },
              ],
            },
          },
        ],
      },
    ],
  },
];

export default routes;
