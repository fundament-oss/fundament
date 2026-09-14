import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { Router } from '@angular/router';
import { NEVER } from 'rxjs';
import { Transport } from '@connectrpc/connect';
import App from './app';
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
async function setUpShell(url: string, arriving: string | null) {
  const navigatedTo: string[] = [];
  await configure([
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
        userOrganizations: signal([ORGANIZATION]),
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
});
