import { Routes } from '@angular/router';
import { authGuard } from './auth.guard';

export const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./login/login.component').then((m) => m.LoginComponent),
  },
  {
    path: 'add-cluster',
    loadComponent: () =>
      import('./add-cluster/add-cluster.component').then((m) => m.AddClusterComponent),
  },
  {
    path: 'add-cluster-nodes',
    loadComponent: () =>
      import('./add-cluster-nodes/add-cluster-nodes.component').then(
        (m) => m.AddClusterNodesComponent,
      ),
  },
  {
    path: 'add-cluster-plugins',
    loadComponent: () =>
      import('./add-cluster-plugins/add-cluster-plugins.component').then(
        (m) => m.AddClusterPluginsComponent,
      ),
  },
  {
    path: 'cluster-nodes',
    loadComponent: () =>
      import('./cluster-nodes/cluster-nodes.component').then((m) => m.ClusterNodesComponent),
  },
  {
    path: 'cluster-plugins',
    loadComponent: () =>
      import('./cluster-plugins/cluster-plugins.component').then((m) => m.ClusterPluginsComponent),
  },
  {
    path: 'projects',
    loadComponent: () => import('./projects/projects.component').then((m) => m.ProjectsComponent),
  },
  {
    path: 'add-cluster-summary',
    loadComponent: () =>
      import('./add-cluster-summary/add-cluster-summary.component').then(
        (m) => m.AddClusterSummaryComponent,
      ),
  },
  {
    path: 'cluster-overview',
    loadComponent: () =>
      import('./cluster-overview/cluster-overview.component').then(
        (m) => m.ClusterOverviewComponent,
      ),
  },
  {
    path: 'project-permissions',
    loadComponent: () =>
      import('./project-permissions/project-permissions.component').then(
        (m) => m.ProjectPermissionsComponent,
      ),
  },
  {
    path: 'project-members',
    loadComponent: () =>
      import('./project-members/project-members.component').then((m) => m.ProjectMembersComponent),
  },
  {
    path: 'plugins',
    loadComponent: () => import('./plugins/plugins.component').then((m) => m.PluginsComponent),
  },
  {
    path: 'profile',
    loadComponent: () => import('./profile/profile.component').then((m) => m.ProfileComponent),
    canActivate: [authGuard],
  },
  {
    path: 'plugins/details',
    loadComponent: () =>
      import('./plugin-details/plugin-details.component').then((m) => m.PluginDetailsComponent),
  },
  {
    path: 'usage',
    loadComponent: () => import('./usage/usage.component').then((m) => m.UsageComponent),
  },
  {
    path: 'tenant',
    loadComponent: () => import('./tenant/tenant.component').then((m) => m.TenantComponent),
  },
  {
    path: '',
    loadComponent: () =>
      import('./dashboard/dashboard.component').then((m) => m.DashboardComponent),
  },
];
