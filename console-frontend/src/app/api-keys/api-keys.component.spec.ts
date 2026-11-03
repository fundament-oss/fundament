import { vi } from 'vitest';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { type Observable, NEVER, Subject, of, throwError } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError, type CallOptions } from '@connectrpc/connect';
import ApiKeysComponent from './api-keys.component';
import { APIKEY } from '../../connect/tokens';
import { NotificationService } from '../notification.service';
import AuthnApiService from '../authn-api.service';
import OrganizationContextService from '../organization-context.service';
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

async function render(client: Record<string, unknown>) {
  TestBed.configureTestingModule({
    imports: [ApiKeysComponent],
    providers: [
      { provide: APIKEY, useValue: client },
      { provide: NotificationService, useValue: { success: vi.fn() } },
      { provide: AuthnApiService, useValue: { currentUser$: of({ id: 'user-1' }) } },
      { provide: OrganizationContextService, useValue: { currentOrganizationId: signal('org-1') } },
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

  it('refuses a malformed duration and an empty name before sending anything', async () => {
    const fixture = await render({ listAPIKeys: () => list() });
    fixture.componentInstance.startCreating();
    fixture.detectChanges();

    const duration = document.body.querySelector<ValidationItem>('#api-key-expires-in-format');
    expect(duration?.test('10 days')).toBe(false);
    expect(duration?.test('5m')).toBe(true);
    expect(duration?.test('1h30m')).toBe(true);

    const name = document.body.querySelector<ValidationItem>('#api-key-name-required');
    expect(name?.test('')).toBe(false);
    expect(name?.test('   ')).toBe(false);
    expect(name?.test('ci')).toBe(true);
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
