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
        path: 'add-cluster',
        loadComponent: () =>
          import('./add-cluster-wizard-layout/add-cluster-wizard-layout.component').then(
            (m) => m.default,
          ),
        data: {
          breadcrumbs: [
            { label: 'Clusters', route: '/' },
            { label: 'Add cluster', route: '/add-cluster' },
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
              breadcrumbs: [{ label: 'Worker nodes' }],
            },
          },
          {
            path: 'plugins',
            loadComponent: () =>
              import('./add-cluster-plugins/add-cluster-plugins.component').then((m) => m.default),
            canActivate: [clusterWizardGuard],
            data: {
              breadcrumbs: [{ label: 'Plugins' }],
            },
          },
          {
            path: 'summary',
            loadComponent: () =>
              import('./add-cluster-summary/add-cluster-summary.component').then((m) => m.default),
            canActivate: [clusterWizardGuard],
            data: {
              breadcrumbs: [{ label: 'Summary' }],
            },
          },
        ],
      },
      {
        path: 'clusters/:id/nodes',
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
        path: 'clusters/:id/plugins',
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
      {
        path: 'clusters/:id/namespaces',
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
        path: 'projects',
        loadComponent: () => import('./projects/projects.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Projects', route: '/projects' }],
        },
      },
      {
        path: 'projects/add',
        loadComponent: () => import('./add-project/add-project.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Projects', route: '/projects' }, { label: 'Add project' }],
        },
      },
      {
        path: 'projects/:id',
        loadComponent: () =>
          import('./project-detail/project-detail.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'General' }],
        },
      },
      {
        path: 'projects/:id/roles',
        loadComponent: () =>
          import('./project-roles/project-roles.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Roles' }],
        },
      },
      {
        path: 'clusters/:id',
        loadComponent: () =>
          import('./cluster-details/cluster-details.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Clusters', route: '/' }, { label: ':clusterName' }],
        },
      },
      {
        path: 'projects/:id/namespaces',
        loadComponent: () => import('./namespaces/namespaces.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Namespaces' }],
        },
      },
      {
        path: 'projects/:id/members',
        loadComponent: () =>
          import('./project-members/project-members.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Members' }],
        },
      },
      {
        path: 'projects/:id/settings',
        loadComponent: () =>
          import('./project-settings/project-settings.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Settings' }],
        },
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
        path: 'usage',
        loadComponent: () => import('./usage/usage.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Usage' }],
        },
      },
      {
        path: 'projects/:id/usage',
        loadComponent: () => import('./usage/usage.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: ':projectName', route: '/projects/:id' }, { label: 'Usage' }],
        },
      },
      {
        path: 'organization',
        loadComponent: () => import('./organization/organization.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Organization' }],
        },
      },
      {
        path: 'organization/members',
        loadComponent: () =>
          import('./organization-members/organization-members.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Organization members' }],
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
        loadComponent: () =>
          import('./plugin-resources/plugin-layout/plugin-layout.component').then((m) => m.default),
        children: [
          {
            path: ':resourceKind',
            loadComponent: () =>
              import('./plugin-resources/resource-list/resource-list.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [{ label: ':pluginDisplayName' }, { label: ':resourceKindLabel' }],
            },
          },
          {
            path: ':resourceKind/create',
            loadComponent: () =>
              import('./plugin-resources/resource-create/resource-create.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [
                { label: ':pluginDisplayName' },
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
                { label: ':pluginDisplayName' },
                {
                  label: ':resourceKindLabel',
                  route: '/plugin-resources/:pluginName/:resourceKind',
                },
                { label: 'Details' },
              ],
            },
          },
        ],
      },
      // Plugin resource routes (project-level)
      {
        path: 'projects/:id/plugin-resources/:pluginName',
        loadComponent: () =>
          import('./plugin-resources/plugin-layout/plugin-layout.component').then((m) => m.default),
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
                { label: ':pluginDisplayName' },
                { label: ':resourceKindLabel' },
              ],
            },
          },
          {
            path: ':resourceKind/create',
            loadComponent: () =>
              import('./plugin-resources/resource-create/resource-create.component').then(
                (m) => m.default,
              ),
            data: {
              breadcrumbs: [
                { label: ':projectName', route: '/projects/:id' },
                { label: ':pluginDisplayName' },
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
                { label: ':pluginDisplayName' },
                {
                  label: ':resourceKindLabel',
                  route: '/projects/:id/plugin-resources/:pluginName/:resourceKind',
                },
                { label: 'Details' },
              ],
            },
          },
        ],
      },
      {
        path: '',
        loadComponent: () => import('./dashboard/dashboard.component').then((m) => m.default),
        data: {
          breadcrumbs: [{ label: 'Clusters' }],
        },
      },
    ],
  },
];

export default routes;
