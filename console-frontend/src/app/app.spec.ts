import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { signal } from '@angular/core';
import { Router, type ActivatedRouteSnapshot } from '@angular/router';
import { NEVER } from 'rxjs';
import { Transport, createRouterTransport } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';
import App, { depthForPath, isOrgAdminOnly, parentOf } from './app';
import OrganizationRoleService from './organization-role.service';
import AuthnApiService from './authn-api.service';
import {
  OrganizationService,
  ListOrganizationsResponseSchema,
  type Organization,
} from '../generated/v1/organization_pb';
import {
  InviteService,
  ListInvitationsResponseSchema,
  AcceptInvitationResponseSchema,
  InvitationSchema,
  type Invitation,
} from '../generated/v1/invite_pb';
import { ConfigService, type AppConfiguration } from './config.service';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';
import PluginRegistryService from './plugin-resources/plugin-registry.service';
import KubeClusterContextService from './plugin-resources/kube-cluster-context.service';
import {
  AUTHN_TRANSPORT,
  ORGANIZATION_TRANSPORT,
  MARKETPLACE_TRANSPORT,
} from '../connect/connect.module';

// The shell injects service clients (MetricsHealthService among them), and every
// client token builds itself from one of these transports. Nothing here makes a
// call, so a transport that would throw if used is enough — the point is that the
// injector can build the component at all.
const unusedTransport = {
  unary: () => Promise.reject(new Error('no transport in tests')),
  stream: () => Promise.reject(new Error('no transport in tests')),
} as unknown as Transport;

// getConfig() throws until loadConfig() has run, which is an APP_INITIALIZER the
// TestBed does not execute. Empty URLs are what an environment with no demo and
// no marketplace deployed looks like.
const config: AppConfiguration = {
  authnApiUrl: '',
  organizationApiUrl: '',
  marketplaceApiUrl: '',
  kubeApiProxyUrl: '',
  pluginProxyUrl: '',
};

async function configure(extraProviders: unknown[] = []) {
  await TestBed.configureTestingModule({
    imports: [App],
    providers: [
      { provide: AUTHN_TRANSPORT, useValue: unusedTransport },
      { provide: ORGANIZATION_TRANSPORT, useValue: unusedTransport },
      { provide: MARKETPLACE_TRANSPORT, useValue: unusedTransport },
      { provide: ConfigService, useValue: { getConfig: () => config } as ConfigService },
      ...(extraProviders as never[]),
    ],
  }).compileComponents();
}

const ORGANIZATION = { id: 'org-1', name: 'acme-corp', alias: 'acme' };

/**
 * The shell with only what settling on an organization touches: an authn
 * service whose token refresh records the call and does nothing else, and a
 * router that records where it was sent rather than going there. `arriving` is
 * the address of an arrival still under way, as `getCurrentNavigation()`
 * reports one; `url` is the page being left, which is all `router.url` knows
 * until that arrival lands.
 *
 * `calls` is the refresh and the organization load in the order they happened:
 * the token has to carry the membership before anything organization-scoped is
 * asked for, and which came first is the only way to see that.
 */
async function setUpShell(url: string, arriving: string | null, extraProviders: unknown[] = []) {
  const navigatedTo: string[] = [];
  const calls: string[] = [];
  const refreshToken = vi.fn(async () => {
    calls.push('refreshToken');
  });
  const userOrganizations = signal<Organization[]>([ORGANIZATION as Organization]);
  await configure([
    ...(extraProviders as never[]),
    { provide: AuthnApiService, useValue: { refreshToken, currentUser$: NEVER } },
    {
      provide: Router,
      useValue: {
        url,
        events: NEVER,
        routerState: { snapshot: { root: null } },
        getCurrentNavigation: () => (arriving ? { finalUrl: arriving } : null),
        serializeUrl: (tree: unknown) => tree as string,
        navigateByUrl: (to: string) => {
          navigatedTo.push(to);
        },
      },
    },
    {
      provide: OrganizationDataService,
      useValue: {
        userOrganizations,
        setUserOrganizations: (orgs: Organization[]) => userOrganizations.set(orgs),
        organizations: signal([ORGANIZATION]),
        clusterSummaries: signal([]),
        members: signal([]),
        membersLoaded: signal(false),
        getOrganizationById: () => ORGANIZATION,
        getProjectById: () => undefined,
        getClusterById: () => undefined,
        loadOrganizationData: async () => {
          calls.push('loadOrganizationData');
        },
      },
    },
    {
      provide: OrganizationContextService,
      useValue: {
        currentOrganizationId: signal<string | null>(null),
        currentOrganizationName: signal<string | null>(null),
        setOrganizationId: () => {},
        setOrganizationName: () => {},
      },
    },
  ]);

  return {
    app: TestBed.createComponent(App).componentInstance,
    navigatedTo,
    refreshToken,
    calls,
  };
}

/**
 * The organization-api as a user with these memberships and invitations sees
 * it. What `handleOrganizationsRecheck` and answering an invitation touch, and
 * nothing else.
 */
function memberOf(organizations: Organization[], invitations: Invitation[] = []): unknown[] {
  const transport = createRouterTransport(({ service }) => {
    service(OrganizationService, {
      listOrganizations: () => create(ListOrganizationsResponseSchema, { organizations }),
    });
    service(InviteService, {
      listInvitations: () => create(ListInvitationsResponseSchema, { invitations }),
      acceptInvitation: () => create(AcceptInvitationResponseSchema, {}),
    });
  });
  return [{ provide: ORGANIZATION_TRANSPORT, useValue: transport }];
}

/**
 * The shell's effects and computed state with a role that is given rather than
 * worked out, and a template that renders nothing: the sidebar would need the
 * whole shell. `role` null is the time before the member list is in.
 */
async function setUpEffects(role: 'admin' | 'viewer' | null = 'admin') {
  const loadPlugins = vi.fn(async () => {});
  const reset = vi.fn();
  const navigatedTo: string[] = [];
  await configure([
    {
      provide: Router,
      useValue: {
        url: '/',
        events: NEVER,
        routerState: { snapshot: { root: null } },
        getCurrentNavigation: () => null,
        serializeUrl: (tree: unknown) => tree as string,
        navigateByUrl: (to: string) => {
          navigatedTo.push(to);
        },
      },
    },
    {
      provide: OrganizationDataService,
      useValue: {
        userOrganizations: signal([]),
        organizations: signal([]),
        clusterSummaries: signal([{ id: 'c2', name: 'production', region: 'nl-ams' }]),
        clustersLoaded: signal(false),
        getOrganizationById: () => undefined,
        getProjectById: (id: string) => (id === 'p1' ? { cluster: { id: 'c1' } } : undefined),
        getClusterById: () => undefined,
      },
    },
    {
      provide: OrganizationContextService,
      useValue: {
        currentOrganizationId: signal<string | null>(null),
        currentOrganizationName: signal<string | null>('acme-corp'),
      },
    },
    {
      provide: OrganizationRoleService,
      useValue: {
        roleKnown: signal(role !== null),
        isOrgAdmin: signal(role === 'admin'),
        isOrgViewer: signal(role === 'viewer'),
      },
    },
    {
      provide: PluginRegistryService,
      useValue: { loadPlugins, reset, allPlugins: signal([]) },
    },
    { provide: KubeClusterContextService, useValue: { onClusterChange: () => {} } },
  ]);
  TestBed.overrideComponent(App, { set: { template: '' } });
  const app = TestBed.createComponent(App).componentInstance;
  return { app, loadPlugins, reset, navigatedTo };
}

describe('App', () => {
  it('should create the app', async () => {
    await configure();
    const fixture = TestBed.createComponent(App);
    const app = fixture.componentInstance;
    expect(app).toBeTruthy();
  });

  it('writes the organization into the address it settled from', async () => {
    const { app, navigatedTo } = await setUpShell('/clusters', null);

    await app.handleOrgPickerSelection(ORGANIZATION.id);

    expect(navigatedTo).toEqual(['/organizations/acme-corp/clusters']);
  });

  it('leaves an arrival that is still under way to land where it was going', async () => {
    // Loading the organization takes a few calls, and the arrival that asked
    // for it can still be waiting on a guard or on its own chunk. Writing the
    // address from `router.url` would cancel it and drop the page it named.
    const { app, navigatedTo } = await setUpShell('/', '/organizations/acme-corp/clusters');

    await app.handleOrgPickerSelection(ORGANIZATION.id);

    expect(navigatedTo).toEqual([]);
  });

  it('sends an arrival that names no organization into the one it settled on', async () => {
    const { app, navigatedTo } = await setUpShell('/', '/clusters');

    await app.handleOrgPickerSelection(ORGANIZATION.id);

    expect(navigatedTo).toEqual(['/organizations/acme-corp/clusters']);
  });

  it('leaves the login page where it is, hand-off and all', async () => {
    // A session turning up while the login page is open would otherwise move
    // it to /organizations/acme-corp/login, which is no page, and the
    // marketplace's hand-off would never be sent back.
    const { app, navigatedTo } = await setUpShell(
      '/login?app=marketplace-registry&path=%2Fmanage',
      null,
    );

    await app.handleOrgPickerSelection(ORGANIZATION.id);

    expect(navigatedTo).toEqual([]);
  });

  // Signing in creates no organization. Someone in none gets the picker's
  // message rather than a blank pane, and stays there.
  it('shows the picker to someone in no organization', async () => {
    const { app, navigatedTo } = await setUpShell('/clusters', null, memberOf([]));

    await app.handleOrganizationsRecheck();

    expect(app.showOrgPicker()).toBe(true);
    expect(app.selectedOrgId()).toBeNull();
    expect(navigatedTo).toEqual([]);
  });

  it('settles on the organization an operator has since added someone to', async () => {
    // The operator ran `funops organization member add` after the shell first
    // loaded; checking again finds the membership and goes in.
    const { app, navigatedTo } = await setUpShell(
      '/clusters',
      null,
      memberOf([ORGANIZATION as Organization]),
    );
    app.showOrgPicker.set(true);

    await app.handleOrganizationsRecheck();

    expect(app.showOrgPicker()).toBe(false);
    expect(app.selectedOrgId()).toBe(ORGANIZATION.id);
    expect(navigatedTo).toEqual(['/organizations/acme-corp/clusters']);
  });

  // The organizations are the database's answer and the token is sign-in's: a
  // membership added in between leaves the two out of step, and loading the
  // organization with the token that still names none is refused outright. So
  // the refresh has to come first, and once is enough.
  it('refreshes the token once, before loading the organization it settles on', async () => {
    const { app, refreshToken, calls } = await setUpShell(
      '/clusters',
      null,
      memberOf([ORGANIZATION as Organization]),
    );
    app.showOrgPicker.set(true);

    await app.handleOrganizationsRecheck();

    expect(calls).toEqual(['refreshToken', 'loadOrganizationData']);
    expect(refreshToken).toHaveBeenCalledTimes(1);
  });

  it('refreshes the token once when an accepted invitation settles the organization', async () => {
    const invitation = create(InvitationSchema, {
      id: 'inv-1',
      organizationId: ORGANIZATION.id,
      organizationAlias: ORGANIZATION.alias,
    });
    const { app, refreshToken, calls } = await setUpShell(
      '/clusters',
      null,
      memberOf([ORGANIZATION as Organization], [invitation]),
    );

    await app.handleAcceptInvitation(invitation);

    expect(calls).toEqual(['refreshToken', 'loadOrganizationData']);
    expect(refreshToken).toHaveBeenCalledTimes(1);
    expect(app.selectedOrgId()).toBe(ORGANIZATION.id);
  });

  it('follows the cluster in the URL when no project is active', async () => {
    const { app, loadPlugins, reset } = await setUpEffects();

    app.currentUrl.set('/organizations/acme-corp/clusters/c2');
    TestBed.tick();
    expect(loadPlugins).toHaveBeenLastCalledWith('c2');

    app.currentUrl.set('/organizations/acme-corp/projects/p1');
    TestBed.tick();
    expect(loadPlugins).toHaveBeenLastCalledWith('c1');

    loadPlugins.mockClear();
    app.currentUrl.set('/organizations/acme-corp/clusters/new');
    TestBed.tick();
    expect(reset).toHaveBeenCalled();
    expect(loadPlugins).not.toHaveBeenCalled();

    app.currentUrl.set('/organizations/acme-corp/clusters/c2');
    TestBed.tick();
    reset.mockClear();
    loadPlugins.mockClear();
    app.currentUrl.set('/organizations/acme-corp/clusters');
    TestBed.tick();
    expect(reset).toHaveBeenCalled();
    expect(loadPlugins).not.toHaveBeenCalled();
  });

  it('loads no cluster plugins for a viewer, who gets no cluster menu', async () => {
    const { app, loadPlugins } = await setUpEffects('viewer');

    app.currentUrl.set('/organizations/acme-corp/clusters/c2');
    TestBed.tick();
    expect(loadPlugins).not.toHaveBeenCalled();

    // A project still brings its cluster's plugins: that menu is theirs.
    app.currentUrl.set('/organizations/acme-corp/projects/p1');
    TestBed.tick();
    expect(loadPlugins).toHaveBeenLastCalledWith('c1');
  });

  it('opens a cluster column for an admin only', async () => {
    const admin = await setUpEffects('admin');
    admin.app.currentUrl.set('/organizations/acme-corp/clusters/c2');
    expect(admin.app.activeCluster()?.name).toBe('production');

    TestBed.resetTestingModule();
    const viewer = await setUpEffects('viewer');
    viewer.app.currentUrl.set('/organizations/acme-corp/clusters/c2');
    expect(viewer.app.activeCluster()).toBeNull();
  });

  it('says "not available" to a viewer on an admin-only page', async () => {
    const { app } = await setUpEffects('viewer');
    app.currentUrl.set('/organizations/acme-corp/quotas');
    app['onAdminOnlyRoute'].set(true);

    expect(app.pageNotAvailable()).toBe(true);
    // Also where the address is a cluster's own: the narrow view has no
    // cluster menu to show a viewer, so the main pane has to carry the message.
    app.stackDepth.set(1);
    expect(app.mainHasContent()).toBe(true);
  });

  it('leads a viewer back from "not available" to the organization, not the cluster', async () => {
    const { app, navigatedTo } = await setUpEffects('viewer');
    // The cluster's own address would turn them away a second time.
    (TestBed.inject(Router) as { url: string }).url =
      '/organizations/acme-corp/clusters/c2/general';
    app['onAdminOnlyRoute'].set(true);

    app.onPaneBack(new Event('back'));

    expect(navigatedTo).toEqual(['/organizations/acme-corp']);
  });

  it('holds an admin-only page back until the role is known', async () => {
    const { app } = await setUpEffects(null);
    app['onAdminOnlyRoute'].set(true);

    expect(app.pageNotAvailable()).toBe(false);
    expect(app.pageAwaitsRole()).toBe(true);
  });

  it('shows an admin the admin-only page', async () => {
    const { app } = await setUpEffects('admin');
    app['onAdminOnlyRoute'].set(true);

    expect(app.pageNotAvailable()).toBe(false);
    expect(app.pageAwaitsRole()).toBe(false);
  });

  it('sends a viewer landing on the clusters to the organization root', async () => {
    const { app, navigatedTo } = await setUpEffects('viewer');

    app.currentUrl.set('/organizations/acme-corp/clusters?present=1');
    TestBed.tick();

    expect(navigatedTo).toEqual(['/organizations/acme-corp?present=1']);
  });

  it('leaves an admin on the clusters', async () => {
    const { app, navigatedTo } = await setUpEffects('admin');

    app.currentUrl.set('/organizations/acme-corp/clusters');
    TestBed.tick();

    expect(navigatedTo).toEqual([]);
  });
});

describe('parentOf', () => {
  it('leads a cluster plugin page to its cluster', () => {
    expect(parentOf('/organizations/acme-corp/clusters/c1/plugin-resources/p/Kind/x?a=b')).toBe(
      '/clusters/c1',
    );
  });

  it('leads every cluster page to its cluster', () => {
    expect(parentOf('/organizations/acme-corp/clusters/c1/nodes')).toBe('/clusters/c1');
    expect(parentOf('/organizations/acme-corp/clusters/c1/general')).toBe('/clusters/c1');
  });

  it('keeps leading project pages to their project and others to the organization', () => {
    expect(parentOf('/organizations/acme-corp/projects/p1/members')).toBe('/projects/p1');
    expect(parentOf('/organizations/acme-corp/clusters/c1')).toBe('/');
    expect(parentOf('/organizations/acme-corp/clusters/new')).toBe('/');
  });
});

describe('depthForPath', () => {
  it('puts a cluster, like a project, at its own menu', () => {
    expect(depthForPath('/organizations/acme-corp/clusters/c1')).toBe(1);
    expect(depthForPath('/organizations/acme-corp/projects/p1')).toBe(1);
  });

  it('puts the pages under a cluster at depth 2', () => {
    expect(depthForPath('/organizations/acme-corp/clusters/c1/nodes')).toBe(2);
    expect(depthForPath('/organizations/acme-corp/clusters')).toBe(2);
  });

  it('keeps the new-cluster sheet over the list rather than a cluster menu', () => {
    expect(depthForPath('/organizations/acme-corp/clusters/new')).toBe(2);
  });

  it('opens on the organization menu', () => {
    expect(depthForPath('/organizations/acme-corp')).toBe(0);
  });
});

describe('isOrgAdminOnly', () => {
  const level = (data: Record<string, unknown>, firstChild: unknown = null) =>
    ({ data, firstChild }) as unknown as ActivatedRouteSnapshot;

  it('finds the mark on any level of the route', () => {
    expect(isOrgAdminOnly(level({}, level({}, level({ orgAdminOnly: true }))))).toBe(true);
    expect(isOrgAdminOnly(level({}, level({ orgAdminOnly: true }, level({}))))).toBe(true);
  });

  it('is false for a route without it', () => {
    expect(isOrgAdminOnly(level({}, level({ pluginScope: 'cluster' })))).toBe(false);
    expect(isOrgAdminOnly(null)).toBe(false);
  });
});
