import { afterEach, describe, expect, it, vi } from 'vitest';
import { TestBed } from '@angular/core/testing';
import { ActivatedRouteSnapshot, Router, RouterStateSnapshot, UrlTree } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import staffAuthGuard, { safeReturnUrl } from './staff-auth.guard';
import StaffSessionService from './staff-session.service';
import {
  AppConfiguration,
  CONFIG_LOADER,
  ConfigService,
  EMPTY_CONFIGURATION,
} from '../config.service';
import { DCIM_AUTHN_CLIENT } from '../../connect/dcim-authn';

const DCIM_AUTHN_URL = 'https://dcim-authn.fundament.localhost:8443';

type Outcome = { id: string; name: string } | 'unauthenticated' | 'unavailable';

async function setup(config: Partial<AppConfiguration>, outcome: Outcome) {
  const getUserInfo = vi.fn(async () => {
    if (outcome === 'unauthenticated') throw new ConnectError('no session', Code.Unauthenticated);
    if (outcome === 'unavailable') throw new ConnectError('down', Code.Unavailable);
    return { user: outcome };
  });

  TestBed.configureTestingModule({
    providers: [
      { provide: CONFIG_LOADER, useValue: async () => ({ ...EMPTY_CONFIGURATION, ...config }) },
      { provide: DCIM_AUTHN_CLIENT, useValue: { getUserInfo } },
    ],
  });
  await TestBed.inject(ConfigService).loadConfig();
  return { getUserInfo, session: TestBed.inject(StaffSessionService) };
}

async function runGuard(url = '/admin/submissions/42') {
  const result = await TestBed.runInInjectionContext(() =>
    staffAuthGuard({} as ActivatedRouteSnapshot, { url } as RouterStateSnapshot),
  );
  return result instanceof UrlTree ? TestBed.inject(Router).serializeUrl(result) : result;
}

describe('staffAuthGuard', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('lets a signed-in reviewer through', async () => {
    await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, { id: 'u1', name: 'Rev Iewer' });

    expect(await runGuard()).toBe(true);
  });

  it('sends a visitor without a session to the login, with the page to come back to', async () => {
    await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, 'unauthenticated');

    expect(await runGuard()).toBe('/login?returnUrl=%2Fadmin%2Fsubmissions%2F42');
  });

  // Nobody can use the backoffice without being identified, so an outage is
  // shown on the login page, as a failed sign-in, rather than as an API error.
  it('sends a visitor to the login when dcim-authn-api does not answer', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, 'unavailable');

    expect(await runGuard('/admin')).toBe('/login?returnUrl=%2Fadmin');
  });

  it('lets the navigation through when there is nowhere to sign in', async () => {
    const { getUserInfo } = await setup({}, 'unauthenticated');

    expect(await runGuard()).toBe(true);
    expect(getUserInfo).not.toHaveBeenCalled();
  });

  // A session that turned up since (a login in DCIM, in another tab) is found
  // on the next navigation rather than the empty answer being kept.
  it('asks again after an empty answer', async () => {
    const { getUserInfo } = await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, 'unauthenticated');

    await runGuard();
    await runGuard();

    expect(getUserInfo).toHaveBeenCalledTimes(2);
  });

  it('asks once for as long as the answer is a reviewer', async () => {
    const { getUserInfo } = await setup(
      { dcimAuthnApiUrl: DCIM_AUTHN_URL },
      { id: 'u1', name: 'Rev Iewer' },
    );

    await runGuard();
    await runGuard();

    expect(getUserInfo).toHaveBeenCalledTimes(1);
  });
});

describe('safeReturnUrl', () => {
  it('keeps a path in this app', () => {
    expect(safeReturnUrl('/admin/submissions/42?tab=files')).toBe(
      '/admin/submissions/42?tab=files',
    );
  });

  it.each([
    ['nothing', null],
    ['an empty string', ''],
    ['another origin', 'https://evil.example/admin'],
    ['a protocol-relative URL', '//evil.example/admin'],
    ['a backslash the browser reads as a slash', '/\\evil.example/admin'],
    ['a relative path', 'admin'],
    ['the login itself', '/login'],
    ['the login with a return URL of its own', '/login?returnUrl=%2Fadmin'],
  ])('falls back to the queue for %s', (_label, returnUrl) => {
    expect(safeReturnUrl(returnUrl)).toBe('/admin');
  });
});

describe('StaffSessionService.login', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('posts the credentials to dcim-authn-api and reads the session back', async () => {
    const { session } = await setup(
      { dcimAuthnApiUrl: `${DCIM_AUTHN_URL}/` },
      { id: 'u1', name: 'Rev Iewer' },
    );
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}'));

    const user = await session.login('rev@example.com', 'secret');

    expect(user).toEqual({ id: 'u1', name: 'Rev Iewer' });
    expect(session.user()).toEqual({ id: 'u1', name: 'Rev Iewer' });
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toBe(`${DCIM_AUTHN_URL}/login/password`);
    expect(init).toMatchObject({ method: 'POST', credentials: 'include' });
    expect(JSON.parse(init?.body as string)).toEqual({
      email: 'rev@example.com',
      password: 'secret',
    });
  });

  it.each([
    [401, 'The email address or password is incorrect.'],
    [404, 'Signing in with a password is not enabled in this environment.'],
    [500, 'The sign-in service did not answer. Try again in a moment.'],
  ])('explains a %i from dcim-authn-api', async (status, message) => {
    const { session } = await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, 'unauthenticated');
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}', { status }));

    await expect(session.login('rev@example.com', 'secret')).rejects.toThrow(message);
    expect(session.user()).toBeNull();
  });

  it('says so when the cookie was set but does not reach this site', async () => {
    const { session } = await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, 'unauthenticated');
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}'));

    await expect(session.login('rev@example.com', 'secret')).rejects.toThrow(
      'You were signed in, but the session did not reach this site.',
    );
  });

  it('says so when dcim-authn-api cannot be reached', async () => {
    const { session } = await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, 'unauthenticated');
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('Failed to fetch'));

    await expect(session.login('rev@example.com', 'secret')).rejects.toThrow(
      'The sign-in service could not be reached. Try again in a moment.',
    );
  });

  // Signed in, but the lookup after it failed: an outage, not a cookie that
  // does not reach this host, so the message must not point at cookies.
  it('tells a failed lookup apart from a session that did not arrive', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { session } = await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, 'unavailable');
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}'));

    await expect(session.login('rev@example.com', 'secret')).rejects.toThrow(
      'You were signed in, but your account could not be looked up. Try again in a moment.',
    );
  });

  // The login page asks for the session as it opens. If that answer is slow
  // and lands after the one login() asked for, it must not sign the reviewer
  // back out.
  it('keeps the reviewer when a lookup from before signing in answers late', async () => {
    const reviewer = { id: 'u1', name: 'Rev Iewer' };
    const { session, getUserInfo } = await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, reviewer);
    let answerEarlyLookup!: () => void;
    getUserInfo.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          answerEarlyLookup = () => reject(new ConnectError('no session', Code.Unauthenticated));
        }),
    );
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}'));

    const early = session.ensureUser();
    await session.login('rev@example.com', 'secret');
    answerEarlyLookup();

    expect(await early).toEqual(reviewer);
    expect(session.user()).toEqual(reviewer);
  });

  it('ignores a lookup that answers after signing out', async () => {
    const reviewer = { id: 'u1', name: 'Rev Iewer' };
    const { session, getUserInfo } = await setup({ dcimAuthnApiUrl: DCIM_AUTHN_URL }, reviewer);
    let answerLookup!: () => void;
    getUserInfo.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          answerLookup = () => resolve({ user: reviewer });
        }),
    );
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}'));

    const lookup = session.ensureUser();
    await session.logout();
    answerLookup();
    await lookup;

    expect(session.user()).toBeNull();
  });
});
