import { inject } from '@angular/core';
import {
  Router,
  type ActivatedRouteSnapshot,
  type CanActivateFn,
  type Routes,
} from '@angular/router';
import authGuard from './auth.guard';
import { inOrganization, organizationOf } from './address';
import OrganizationContextService from './organization-context.service';
import { OverlayService } from './overlay.service';

/** The organization the address names, read from wherever it sits above. */
const organizationNameOf = (route: ActivatedRouteSnapshot): string | null =>
  route.pathFromRoot.map((step) => step.params['organizationName']).find(Boolean) ?? null;

/**
 * Keeps an address for a sheet the shell owns. The sheet is not a page, so the
 * route has nothing to render: it opens the sheet and sends you to the page the
 * thing lands on, with the sheet already over it. Opened from the toolbar
 * instead, the same sheet appears over whatever page you were reading, and no
 * address is involved at all.
 */
const opensSheet =
  (
    open: (overlays: OverlayService, route: ActivatedRouteSnapshot) => void,
    to: (route: ActivatedRouteSnapshot) => string,
  ): CanActivateFn =>
  (route) => {
    const overlays = inject(OverlayService);
    const target = inOrganization(organizationNameOf(route), to(route));
    open(overlays, route);
    // The shell closes its sheets on arrival somewhere else, so it has to know
    // that this one arrival belongs to the sheet. Waiting for the navigation
    // instead would leave the sheet shut whenever the redirect lands on the
    // page you are already on: Angular skips that navigation, so the event
    // never comes and the sheet would open on whatever page you went to next.
    overlays.openedFor.set(target);
    return inject(Router).parseUrl(target);
  };

/** The three that belong to no page at all: they open over the home pane. */
const opensOverlay = (sheet: 'profile' | 'apiKeys' | 'newProject'): CanActivateFn =>
  opensSheet(
    (overlays) => overlays[sheet].set(true),
    () => '/',
  );

/**
 * The address names the organization, so the console follows the address rather
 * than the other way round. Its name is what stands there; the shell looks up
 * which organization that is and sets the id the API knows it by.
 */
const organizationFromAddress: CanActivateFn = (route) => {
  const name = route.paramMap.get('organizationName');
  if (name) inject(OrganizationContextService).setOrganizationName(name);
  return true;
};

/**
 * An address without an organization: the bare root the browser opens on, or a
 * link written before the organization was part of one. Both go into the
 * organization this browser was last in.
 *
 * With none to go on the shell is still choosing, or asking which one, and it
 * navigates as soon as it knows. An address that does name an organization and
 * still ends up here matched nothing below it, and is left alone rather than
 * prefixed a second time.
 */
const intoCurrentOrganization: CanActivateFn = (route, state) => {
  if (organizationOf(state.url)) return true;

  const last = OrganizationContextService.getStoredOrganizationName();
  return last ? inject(Router).parseUrl(inOrganization(last, state.url)) : true;
};

const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./login/login.component').then((m) => m.default),
  },
  {
    // Everything below is read inside one organization, and says so. The pages
    // that are about the organization itself sit here as well, next to the
    // clusters and the projects rather than under a second word for the same
    // thing: this address already names it.
    path: 'organizations/:organizationName',
    canActivate: [authGuard, organizationFromAddress],
    children: [
      {
        // Nothing in the main pane: the app opens on a choice, not on a list it
        // picked for you. A componentless route so the URL still matches while
        // the outlet stays empty.
        path: '',
        children: [],
      },
      {
        path: 'general',
        loadComponent: () =>
          import('./organization-settings/organization-settings.component').then((m) => m.default),
      },
      {
        path: 'members',
        loadComponent: () =>
          import('./organization-members/organization-members.component').then((m) => m.default),
        // A route of its own so the reference can be linked to, shared and
        // closed with the back button; a child so the list stays mounted behind
        // the sheet.
        children: [
          {
            path: 'invite',
            canActivate: [
              opensSheet(
                (o) => o.inviteMember.set(true),
                () => '/members',
              ),
            ],
            children: [],
          },
          {
            path: 'permissions',
            loadComponent: () =>
              import('./permissions-sheet/permissions-sheet.component').then((m) => m.default),
          },
        ],
      },
      {
        path: 'limits',
        loadComponent: () =>
          import('./organization-limits/organization-limits.component').then((m) => m.default),
      },
      {
        path: 'clusters',
        loadComponent: () => import('./dashboard/dashboard.component').then((m) => m.default),
        // The form is a sheet the shell owns, so the address only opens it.
        children: [
          {
            path: 'new',
            canActivate: [
              opensSheet(
                (o) => o.newCluster.set(true),
                () => '/clusters',
              ),
            ],
            children: [],
          },
          { path: 'new/**', redirectTo: 'new' },
        ],
      },
      {
        path: 'projects/new',
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
            // Before `:name`, or a namespace called "new" would swallow it.
            path: 'new',
            canActivate: [
              opensSheet(
                (o, route) => o.newNamespace.set(route.parent?.params['id'] ?? null),
                (route) => `/projects/${route.parent?.params['id']}/namespaces`,
              ),
            ],
            children: [],
          },
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
            // Before `:memberId`, or a member called "add" would swallow it.
            path: 'add',
            canActivate: [
              opensSheet(
                (o, route) => o.addProjectMember.set(route.parent?.params['id'] ?? null),
                (route) => `/projects/${route.parent?.params['id']}/members`,
              ),
            ],
            children: [],
          },
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
  {
    path: '**',
    canActivate: [intoCurrentOrganization],
    children: [],
  },
];

export default routes;
