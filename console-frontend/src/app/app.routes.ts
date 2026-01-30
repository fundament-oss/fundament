import { Routes } from '@angular/router';
import { authGuard } from './auth.guard';
import { clusterWizardGuard } from './add-cluster-wizard-layout/cluster-wizard.guard';

export const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./login/login.component').then((m) => m.LoginComponent),
  },
  {
    path: 'add-cluster',
    loadComponent: () =>
      import('./add-cluster-wizard-layout/add-cluster-wizard-layout.component').then(
        (m) => m.AddClusterWizardLayoutComponent,
      ),
    children: [
      {
        path: '',
        loadComponent: () =>
          import('./add-cluster/add-cluster.component').then((m) => m.AddClusterComponent),
        canActivate: [clusterWizardGuard],
      },
      {
        path: 'nodes',
        loadComponent: () =>
          import('./add-cluster-nodes/add-cluster-nodes.component').then(
            (m) => m.AddClusterNodesComponent,
          ),
        canActivate: [clusterWizardGuard],
      },
      {
        path: 'plugins',
        loadComponent: () =>
          import('./add-cluster-plugins/add-cluster-plugins.component').then(
            (m) => m.AddClusterPluginsComponent,
          ),
        canActivate: [clusterWizardGuard],
      },
      {
        path: 'summary',
        loadComponent: () =>
          import('./add-cluster-summary/add-cluster-summary.component').then(
            (m) => m.AddClusterSummaryComponent,
          ),
        canActivate: [clusterWizardGuard],
      },
    ],
  },
  {
    path: 'clusters/:id/nodes',
    loadComponent: () =>
      import('./cluster-nodes/cluster-nodes.component').then((m) => m.ClusterNodesComponent),
  },
  {
    path: 'clusters/:id/plugins',
    loadComponent: () =>
      import('./cluster-plugins/cluster-plugins.component').then((m) => m.ClusterPluginsComponent),
  },
  {
    path: 'projects',
    loadComponent: () => import('./projects/projects.component').then((m) => m.ProjectsComponent),
  },
  {
    path: 'projects/add',
    loadComponent: () =>
      import('./add-project/add-project.component').then((m) => m.AddProjectComponent),
  },
  {
    path: 'projects/:id',
    loadComponent: () =>
      import('./project-detail/project-detail.component').then((m) => m.ProjectDetailComponent),
  },
  {
    path: 'clusters/:id',
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
    path: 'plugins/:id',
    loadComponent: () =>
      import('./plugin-details/plugin-details.component').then((m) => m.PluginDetailsComponent),
  },
  {
    path: 'usage',
    loadComponent: () => import('./usage/usage.component').then((m) => m.UsageComponent),
  },
  {
    path: 'organization',
    loadComponent: () =>
      import('./organization/organization.component').then((m) => m.OrganizationComponent),
  },
  {
    path: 'organization/members',
    loadComponent: () =>
      import('./organization-members/organization-members.component').then(
        (m) => m.OrganizationMembersComponent,
      ),
  },
  {
    path: '',
    loadComponent: () =>
      import('./dashboard/dashboard.component').then((m) => m.DashboardComponent),
    canActivate: [authGuard],
  },
];
