import { vi } from 'vitest';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { type Observable, BehaviorSubject, NEVER, Subject, of, throwError } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError, type CallOptions } from '@connectrpc/connect';
import ApiKeysComponent from './api-keys.component';
import { APIKEY } from '../../connect/tokens';
import { NotificationService } from '../notification.service';
import AuthnApiService from '../authn-api.service';
import OrganizationContextService from '../organization-context.service';
import { isoDate } from './expiry-options';
import {
  type APIKey,
  type ListAPIKeysResponse,
  APIKeySchema,
  CreateAPIKeyResponseSchema,
  DeleteAPIKeyResponseSchema,
  ListAPIKeysResponseSchema,
} from '../../generated/v1/apikey_pb';

type ValidationItem = HTMLElement & { test(value: string): boolean | null };

const key = create(APIKeySchema, { id: 'key-1', name: 'ci', tokenPrefix: 'fun_ab12' });

const list = (...apiKeys: APIKey[]) => of(create(ListAPIKeysResponseSchema, { apiKeys }));

const listWithMaximum = (maxExpiresIn: string) =>
  of(create(ListAPIKeysResponseSchema, { apiKeys: [], maxExpiresIn }));

const created = create(CreateAPIKeyResponseSchema, {
  id: 'key-1',
  token: 'fun_ab12-secret',
  tokenPrefix: 'fun_ab12',
});

/** A create the idempotency layer takes as done: the server says so in a header. */
const createdCall = (_request: unknown, options?: CallOptions) => {
  options?.onHeader?.(new Headers({ 'Idempotency-Status': 'completed' }));
  return of(created);
};

async function render(
  client: Record<string, unknown>,
  {
    user$ = of<{ id: string } | null>({ id: 'user-1' }),
    organizationId = signal<string | null>('org-1'),
  } = {},
) {
  TestBed.configureTestingModule({
    imports: [ApiKeysComponent],
    providers: [
      { provide: APIKEY, useValue: client },
      { provide: NotificationService, useValue: { success: vi.fn() } },
      { provide: AuthnApiService, useValue: { currentUser$: user$ } },
      { provide: OrganizationContextService, useValue: { currentOrganizationId: organizationId } },
    ],
  });
  const fixture = TestBed.createComponent(ApiKeysComponent);
  fixture.componentRef.setInput('show', true);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return fixture;
}

/** The sheet portals itself to the body, so its content is looked up there. */
const banner = (text: string) =>
  [...document.body.querySelectorAll('nldd-banner')].find((el) => el.getAttribute('text') === text);

async function submit(component: ApiKeysComponent, name: string) {
  component.startCreating();
  component.newKeyName.set(name);
  await component.createApiKey(new Event('submit', { cancelable: true }));
}

describe('ApiKeysComponent', () => {
  // A test that fakes the clock and fails before it puts it back must not
  // leave the rest on that clock.
  afterEach(() => {
    vi.useRealTimers();
  });

  it('drops the token and the half-filled form when the sheet closes', async () => {
    const listAPIKeys = vi.fn(() => list(key));
    const fixture = await render({ listAPIKeys, createAPIKey: createdCall });
    const { componentInstance: component } = fixture;

    await submit(component, 'ci');
    fixture.detectChanges();
    expect(banner('API key created')).toBeDefined();

    component.startCreating();
    component.newKeyName.set('half');
    fixture.componentRef.setInput('show', false);
    fixture.componentRef.setInput('show', true);
    await fixture.whenStable();
    fixture.detectChanges();

    expect(component.createdToken()).toBeNull();
    expect(component.isCreating()).toBe(false);
    expect(component.newKeyName()).toBe('');
    expect(banner('API key created')).toBeUndefined();
    // Once for the open, once after the create and once for the reopen.
    expect(listAPIKeys).toHaveBeenCalledTimes(3);
  });

  it('drops the token when its key is deleted', async () => {
    const listAPIKeys = vi.fn(() => list(key));
    const fixture = await render({
      listAPIKeys,
      createAPIKey: createdCall,
      deleteAPIKey: () => of(create(DeleteAPIKeyResponseSchema, {})),
    });
    const { componentInstance: component } = fixture;
    await submit(component, 'ci');
    expect(component.createdToken()).toBe('fun_ab12-secret');

    listAPIKeys.mockImplementation(() => list());
    component.openDeleteModal('key-1', 'ci');
    await component.confirmDelete();
    fixture.detectChanges();

    expect(component.createdToken()).toBeNull();
    expect(banner('API key created')).toBeUndefined();
  });

  it('drops the token when a later list no longer holds its key', async () => {
    const listAPIKeys = vi.fn(() => list(key));
    const fixture = await render({ listAPIKeys, createAPIKey: createdCall });
    const { componentInstance: component } = fixture;
    await submit(component, 'ci');

    // Deleted elsewhere, through functl for instance.
    listAPIKeys.mockImplementation(() => list());
    await component.loadApiKeys();

    expect(component.createdToken()).toBeNull();
  });

  it('keeps the token when a list asked for before the create answers late', async () => {
    const stale = new Subject<ListAPIKeysResponse>();
    const listAPIKeys = vi.fn<() => Observable<ListAPIKeysResponse>>(() => stale);
    const fixture = await render({ listAPIKeys, createAPIKey: createdCall });
    const { componentInstance: component } = fixture;

    listAPIKeys.mockImplementation(() => list(key));
    await submit(component, 'ci');
    stale.next(create(ListAPIKeysResponseSchema, { apiKeys: [] }));
    stale.complete();
    await fixture.whenStable();

    expect(component.createdToken()).toBe('fun_ab12-secret');
    expect(component.apiKeys().map((apiKey) => apiKey.id)).toEqual(['key-1']);
  });

  it('puts a taken name under the Name field, not in the banner', async () => {
    const fixture = await render({
      listAPIKeys: () => list(),
      createAPIKey: () =>
        throwError(
          () => new ConnectError('an API key with this name already exists', Code.AlreadyExists),
        ),
    });
    const { componentInstance: component } = fixture;
    await submit(component, 'ci');
    fixture.detectChanges();

    expect(component.error()).toBeNull();
    expect(component.newKeyNameError()).toBe(
      'An API key with this name already exists in this organization, possibly made by another member.',
    );
    const name = document.body.querySelector('#name');
    expect(name?.getAttribute('unmet')).toBe('api-key-name-taken');
    expect(document.body.querySelector('#api-key-name-taken')?.textContent?.trim()).toBe(
      component.newKeyNameError(),
    );
  });

  it('shows any other refusal without the Connect code in front of it', async () => {
    const fixture = await render({
      listAPIKeys: () => list(),
      createAPIKey: () =>
        throwError(() => new ConnectError('the database is unavailable', Code.Unavailable)),
    });
    const { componentInstance: component } = fixture;
    await submit(component, 'ci');

    expect(component.error()).toBe('Failed to create API key: the database is unavailable');
  });

  it('drops a list asked for by the previous organization', async () => {
    const organizationId = signal<string | null>('org-1');
    const late = new Subject<ListAPIKeysResponse>();
    const listAPIKeys = vi.fn<() => Observable<ListAPIKeysResponse>>(() => list(key));
    const fixture = await render({ listAPIKeys }, { organizationId });
    const { componentInstance: component } = fixture;

    // Out for org-1 when the sheet closes and the organization changes.
    listAPIKeys.mockImplementation(() => late);
    component.loadApiKeys();
    fixture.componentRef.setInput('show', false);
    organizationId.set('org-2');
    fixture.detectChanges();
    late.next(create(ListAPIKeysResponseSchema, { apiKeys: [key] }));
    late.complete();
    await fixture.whenStable();

    expect(component.apiKeys()).toEqual([]);
  });

  it('keeps no token from a create that finishes after a logout', async () => {
    const user$ = new BehaviorSubject<{ id: string } | null>({ id: 'user-1' });
    const response = new Subject<typeof created>();
    const fixture = await render(
      {
        listAPIKeys: () => list(key),
        createAPIKey: (_request: unknown, options?: CallOptions) => {
          options?.onHeader?.(new Headers({ 'Idempotency-Status': 'completed' }));
          return response;
        },
      },
      { user$ },
    );
    const { componentInstance: component } = fixture;
    component.startCreating();
    component.newKeyName.set('ci');

    const pending = component.createApiKey(new Event('submit', { cancelable: true }));
    user$.next(null);
    response.next(created);
    response.complete();
    await pending;

    expect(component.createdToken()).toBeNull();
  });

  it('keeps a late refusal of a name off the name typed since', async () => {
    const response = new Subject<typeof created>();
    const fixture = await render({ listAPIKeys: () => list(), createAPIKey: () => response });
    const { componentInstance: component } = fixture;
    component.startCreating();
    component.newKeyName.set('ci');

    const pending = component.createApiKey(new Event('submit', { cancelable: true }));
    component.newKeyName.set('ci-2');
    response.error(
      new ConnectError('an API key with this name already exists', Code.AlreadyExists),
    );
    await pending;

    expect(component.newKeyNameError()).toBeNull();
  });

  it('holds a rule that refuses an empty name', async () => {
    const fixture = await render({ listAPIKeys: () => list() });
    fixture.componentInstance.startCreating();
    fixture.detectChanges();

    const name = document.body.querySelector<ValidationItem>('#api-key-name-required');
    expect(name?.test('')).toBe(false);
    expect(name?.test('   ')).toBe(false);
    expect(name?.test('ci')).toBe(true);
  });

  it('creates a key with the longest lifetime the server allows unless told otherwise', async () => {
    const createAPIKey = vi.fn(createdCall);
    const listAPIKeys = () => listWithMaximum('8760h');
    const fixture = await render({ listAPIKeys, createAPIKey });

    await submit(fixture.componentInstance, 'ci');

    expect(createAPIKey.mock.calls[0][0]).toMatchObject({ expiresIn: '8760h' });
  });

  it('creates a key that never expires when the server has no maximum', async () => {
    const createAPIKey = vi.fn(createdCall);
    const fixture = await render({ listAPIKeys: () => list(), createAPIKey });

    await submit(fixture.componentInstance, 'ci');

    expect(createAPIKey.mock.calls[0][0]).toMatchObject({ expiresIn: '' });
  });

  it('sends the chosen preset, and a picked day as the time until its end', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 10, 18, 14, 30, 0));
    const createAPIKey = vi.fn(createdCall);
    const fixture = await render({ listAPIKeys: () => listWithMaximum('8760h'), createAPIKey });
    const { componentInstance: component } = fixture;

    component.startCreating();
    component.newKeyName.set('ci');
    component.newKeyExpiryChoice.set('720h');
    await component.createApiKey(new Event('submit', { cancelable: true }));

    component.startCreating();
    component.newKeyName.set('ci-2');
    component.newKeyExpiryChoice.set('custom');
    component.newKeyExpiresOn.set('2026-11-19');
    await component.createApiKey(new Event('submit', { cancelable: true }));
    vi.useRealTimers();

    expect(createAPIKey.mock.calls[0][0]).toMatchObject({ expiresIn: '720h' });
    // From 14:30 to 23:59:59 the next day.
    expect(createAPIKey.mock.calls[1][0]).toMatchObject({
      expiresIn: `${(24 + 9) * 3600 + 29 * 60 + 59}s`,
    });
  });

  it('holds the create back, and says why, while no day is picked under custom', async () => {
    const createAPIKey = vi.fn(createdCall);
    const fixture = await render({ listAPIKeys: () => listWithMaximum('8760h'), createAPIKey });
    const { componentInstance: component } = fixture;

    component.startCreating();
    component.newKeyName.set('ci');
    component.newKeyExpiryChoice.set('custom');
    await component.createApiKey(new Event('submit', { cancelable: true }));
    fixture.detectChanges();

    expect(createAPIKey).not.toHaveBeenCalled();
    expect(component.expiresOnUnmet()).toBe('api-key-expires-on-required');
    const picker = document.body.querySelector('#expiresOn');
    expect(picker?.getAttribute('unmet')).toBe('api-key-expires-on-required');
  });

  it('offers no day in the calendar past the one the maximum runs out on', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 10, 18, 14, 30, 0));
    const fixture = await render({ listAPIKeys: () => listWithMaximum('720h') });
    const { componentInstance: component } = fixture;
    component.startCreating();
    component.newKeyExpiryChoice.set('custom');
    fixture.detectChanges();
    vi.useRealTimers();

    const picker = document.body.querySelector('#expiresOn');
    expect(picker?.getAttribute('max')).toBe('2026-12-18');
    expect(picker?.getAttribute('min')).toBe('today');
  });

  it('counts the last day in the calendar from when the form opens, not from an earlier one', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 10, 18, 14, 30, 0));
    const fixture = await render({ listAPIKeys: () => listWithMaximum('720h') });
    const { componentInstance: component } = fixture;
    component.startCreating();
    component.newKeyExpiryChoice.set('custom');
    fixture.detectChanges();
    component.cancelCreating();
    fixture.detectChanges();

    vi.setSystemTime(new Date(2026, 10, 20, 9, 0, 0));
    component.startCreating();
    component.newKeyExpiryChoice.set('custom');
    fixture.detectChanges();
    vi.useRealTimers();

    expect(component.lastExpiryDate()).toBe('2026-12-20');
  });

  it('puts a refused custom day under its field, and a refused preset in the banner', async () => {
    const refusal = new ConnectError(
      'validation error:\n - expires_in: must be at most 720h',
      Code.InvalidArgument,
    );
    const fixture = await render({
      listAPIKeys: () => listWithMaximum('8760h'),
      createAPIKey: () => throwError(() => refusal),
    });
    const { componentInstance: component } = fixture;

    component.startCreating();
    component.newKeyName.set('ci');
    component.newKeyExpiryChoice.set('custom');
    component.newKeyExpiresOn.set(isoDate(new Date(Date.now() + 60 * 86_400_000)));
    await component.createApiKey(new Event('submit', { cancelable: true }));
    expect(component.newKeyExpiresOnError()).toBe('Must be within 30 days from now.');
    expect(component.error()).toBeNull();

    component.newKeyExpiryChoice.set('8760h');
    await component.createApiKey(new Event('submit', { cancelable: true }));
    expect(component.error()).toBe('This lifetime is not allowed. The longest allowed is 30 days.');
  });

  it('refetches the maximum when a picked day is refused, so the calendar stops where it holds', async () => {
    let maximum = '8760h';
    const listAPIKeys = vi.fn(() => listWithMaximum(maximum));
    const fixture = await render({
      listAPIKeys,
      createAPIKey: () =>
        throwError(
          () =>
            new ConnectError(
              'validation error:\n - expires_in: must be at most 720h',
              Code.InvalidArgument,
            ),
        ),
    });
    const { componentInstance: component } = fixture;
    const fetches = listAPIKeys.mock.calls.length;

    component.startCreating();
    component.newKeyName.set('ci');
    component.newKeyExpiryChoice.set('custom');
    component.newKeyExpiresOn.set(isoDate(new Date(Date.now() + 60 * 86_400_000)));
    maximum = '720h';
    await component.createApiKey(new Event('submit', { cancelable: true }));
    await fixture.whenStable();

    expect(listAPIKeys.mock.calls.length).toBe(fetches + 1);
    expect(component.lastExpiryDate()).toBe(isoDate(new Date(Date.now() + 30 * 86_400_000)));
    expect(component.newKeyExpiresOnError()).toBe('Must be within 30 days from now.');
  });

  it('says a refusal it cannot word in days as the server wrote it', async () => {
    const fixture = await render({
      listAPIKeys: () => listWithMaximum('8760h'),
      createAPIKey: () =>
        throwError(
          () =>
            new ConnectError(
              'validation error:\n - expires_in: must be positive',
              Code.InvalidArgument,
            ),
        ),
    });
    const { componentInstance: component } = fixture;

    component.startCreating();
    component.newKeyName.set('ci');
    component.newKeyExpiryChoice.set('custom');
    component.newKeyExpiresOn.set(isoDate(new Date(Date.now() + 86_400_000)));
    await component.createApiKey(new Event('submit', { cancelable: true }));

    expect(component.newKeyExpiresOnError()).toBe('Must be positive.');
  });

  it('refetches the maximum when a preset is refused, and drops the pick it took away', async () => {
    let maximum = '8760h';
    const listAPIKeys = vi.fn(() => listWithMaximum(maximum));
    const fixture = await render({
      listAPIKeys,
      createAPIKey: () =>
        throwError(
          () =>
            new ConnectError(
              'validation error:\n - expires_in: must be at most 720h',
              Code.InvalidArgument,
            ),
        ),
    });
    const { componentInstance: component } = fixture;
    const fetches = listAPIKeys.mock.calls.length;

    component.startCreating();
    component.newKeyName.set('ci');
    component.newKeyExpiryChoice.set('8760h');
    maximum = '720h';
    await component.createApiKey(new Event('submit', { cancelable: true }));
    await fixture.whenStable();

    expect(listAPIKeys.mock.calls.length).toBe(fetches + 1);
    expect(component.expiryOptions().map((option) => option.value)).toEqual([
      '24h',
      '168h',
      '720h',
      'custom',
    ]);
    expect(component.selectedExpiry()).toBe('720h');
    expect(component.expiresInToSend()).toBe('720h');
  });

  it('drops a refusal of a lifetime that has been changed since it was sent', async () => {
    const response = new Subject<never>();
    const fixture = await render({
      listAPIKeys: () => listWithMaximum('8760h'),
      createAPIKey: () => response,
    });
    const { componentInstance: component } = fixture;

    component.startCreating();
    component.newKeyName.set('ci');
    component.newKeyExpiryChoice.set('custom');
    component.newKeyExpiresOn.set(isoDate(new Date(Date.now() + 60 * 86_400_000)));
    const pending = component.createApiKey(new Event('submit', { cancelable: true }));
    component.newKeyExpiryChoice.set('720h');
    response.error(
      new ConnectError(
        'validation error:\n - expires_in: must be at most 720h',
        Code.InvalidArgument,
      ),
    );
    await pending;

    expect(component.newKeyExpiresOnError()).toBeNull();
    expect(component.error()).toBeNull();
  });

  it('offers no never, and does not send it, before the list has said there is no maximum', async () => {
    const fixture = await render({ listAPIKeys: () => NEVER });
    const { componentInstance: component } = fixture;

    component.startCreating();

    expect(component.expiryOptions().map((option) => option.value)).not.toContain('never');
    expect(component.expiresInToSend()).toBe('8760h');
  });

  it('sends one create however often the form is submitted while it is out', async () => {
    const createAPIKey = vi.fn(() => NEVER);
    const fixture = await render({ listAPIKeys: () => list(), createAPIKey });
    const { componentInstance: component } = fixture;
    component.startCreating();
    component.newKeyName.set('ci');

    // The first never settles: its call is still out when the second arrives.
    component.createApiKey(new Event('submit', { cancelable: true }));
    await component.createApiKey(new Event('submit', { cancelable: true }));

    expect(createAPIKey).toHaveBeenCalledTimes(1);
    expect(component.creating()).toBe(true);
  });
});
