import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { Router } from '@angular/router';
import { NEVER } from 'rxjs';
import { Transport, createRouterTransport } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';
import App from './app';
import AuthnApiService from './authn-api.service';
import {
  OrganizationService,
  ListOrganizationsResponseSchema,
  type Organization,
} from '../generated/v1/organization_pb';
import { InviteService, ListInvitationsResponseSchema } from '../generated/v1/invite_pb';
import { ConfigService, type AppConfiguration } from './config.service';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';
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
 * The shell with only what settling on an organization touches, and a router
 * that records where it was sent rather than going there. `arriving` is the
 * address of an arrival still under way, as `getCurrentNavigation()` reports
 * one; `url` is the page being left, which is all `router.url` knows until that
 * arrival lands.
 */
async function setUpShell(url: string, arriving: string | null, extraProviders: unknown[] = []) {
  const navigatedTo: string[] = [];
  const userOrganizations = signal<Organization[]>([ORGANIZATION as Organization]);
  await configure([
    ...(extraProviders as never[]),
    {
      provide: Router,
      useValue: {
        url,
        events: NEVER,
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
        getOrganizationById: () => ORGANIZATION,
        getProjectById: () => undefined,
        getClusterById: () => undefined,
        loadOrganizationData: async () => {},
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

  return { app: TestBed.createComponent(App).componentInstance, navigatedTo };
}

/**
 * The organization-api as a user with these memberships sees it, and an authn
 * service whose token refresh is a no-op. What `handleOrganizationsRecheck`
 * touches, and nothing else.
 */
function memberOf(organizations: Organization[]): unknown[] {
  const transport = createRouterTransport(({ service }) => {
    service(OrganizationService, {
      listOrganizations: () => create(ListOrganizationsResponseSchema, { organizations }),
    });
    service(InviteService, {
      listInvitations: () => create(ListInvitationsResponseSchema, { invitations: [] }),
    });
  });
  return [
    { provide: ORGANIZATION_TRANSPORT, useValue: transport },
    { provide: AuthnApiService, useValue: { refreshToken: async () => {} } },
  ];
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
});
