import { afterEach, describe, expect, it, vi } from 'vitest';
import { TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import OrganizationContextService from './organization-context.service';
import {
  AppConfiguration,
  CONFIG_LOADER,
  ConfigService,
  EMPTY_CONFIGURATION,
} from './config.service';
import { AUTHN_CLIENT } from '../connect/authn';
import { ORGANIZATION_CLIENT } from '../connect/organization';

const STORAGE_KEY = 'marketplace_selected_organization_id';
const ACME = '11111111-1111-4111-8111-111111111111';
const GLOBEX = '22222222-2222-4222-8222-222222222222';
const STRANGER = '33333333-3333-4333-8333-333333333333';

async function setUp(
  config: Partial<AppConfiguration>,
  listOrganizations: () => unknown,
): Promise<OrganizationContextService> {
  TestBed.configureTestingModule({
    providers: [
      {
        provide: CONFIG_LOADER,
        useValue: async () => ({
          ...EMPTY_CONFIGURATION,
          authnApiUrl: 'https://authn.test',
          ...config,
        }),
      },
      {
        provide: AUTHN_CLIENT,
        useValue: { getUserInfo: () => of({ user: { organizationIds: [ACME, GLOBEX] } }) },
      },
      { provide: ORGANIZATION_CLIENT, useValue: { listOrganizations } },
    ],
  });
  await TestBed.inject(ConfigService).loadConfig();
  return TestBed.inject(OrganizationContextService);
}

describe('OrganizationContextService.forgetStoredOrganization', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('forgets the stored selection and leaves the signals alone', async () => {
    const context = await setUp({}, vi.fn());
    await context.ensureOrganizationId();
    context.setOrganizationId(GLOBEX);

    context.forgetStoredOrganization();

    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
    // Emptying these would reload the plugin list against the ended session.
    expect(context.organizationId()).toBe(GLOBEX);
    expect(context.organizations()).toHaveLength(2);
  });

  it('does not throw where storage is blocked', async () => {
    const context = await setUp({}, vi.fn());
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });

    expect(() => context.forgetStoredOrganization()).not.toThrow();
  });
});

describe('OrganizationContextService organization names', () => {
  it("names the session's organizations, in membership order, and nothing else", async () => {
    const context = await setUp({ organizationApiUrl: 'https://organization.test' }, () =>
      of({
        organizations: [
          { id: GLOBEX, name: 'globex', alias: '' },
          { id: STRANGER, name: 'stranger', alias: 'Stranger' },
          { id: ACME, name: 'acme', alias: 'Acme Gemeente' },
        ],
      }),
    );

    await context.ensureOrganizationId();
    await vi.waitFor(() => expect(context.organizations()[0].named).toBe(true));

    expect(context.organizations()).toEqual([
      { id: ACME, label: 'Acme Gemeente', named: true },
      { id: GLOBEX, label: 'globex', named: true },
    ]);
    expect(context.activeOrganization()?.label).toBe('Acme Gemeente');
  });

  it('keeps shortened ids when organization-api is not configured', async () => {
    const listOrganizations = vi.fn();
    const context = await setUp({}, listOrganizations);

    await context.ensureOrganizationId();

    expect(listOrganizations).not.toHaveBeenCalled();
    expect(context.organizations()).toEqual([
      { id: ACME, label: '11111111…', named: false },
      { id: GLOBEX, label: '22222222…', named: false },
    ]);
  });

  it('does not hold up the active organization on a failing lookup', async () => {
    const context = await setUp({ organizationApiUrl: 'https://organization.test' }, () =>
      throwError(() => new Error('unreachable')),
    );

    await expect(context.ensureOrganizationId()).resolves.toBe(ACME);
    expect(context.activeOrganization()).toEqual({ id: ACME, label: '11111111…', named: false });
  });
});
