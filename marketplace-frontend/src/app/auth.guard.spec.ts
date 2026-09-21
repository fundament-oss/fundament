import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TestBed } from '@angular/core/testing';
import { APP_BASE_HREF, LocationStrategy, PathLocationStrategy } from '@angular/common';
import { ActivatedRouteSnapshot, RouterStateSnapshot } from '@angular/router';
import authGuard from './auth.guard';
import SessionService from './session.service';
import {
  AppConfiguration,
  CONFIG_LOADER,
  ConfigService,
  EMPTY_CONFIGURATION,
} from './config.service';
import { AUTHN_CLIENT } from '../connect/authn';

const AUTHN_API_URL = 'https://authn.fundament.localhost:8443';
const CONSOLE_URL = 'https://console.fundament.localhost:8443';

type SessionOutcome = { user: { organizationIds: string[] } | undefined } | 'unauthenticated';

async function runGuard(
  config: Partial<AppConfiguration>,
  outcome: SessionOutcome,
  baseHref = '/',
): Promise<{ allowed: boolean; redirectedTo?: string }> {
  TestBed.configureTestingModule({
    providers: [
      // The real strategy, not the location mocks: the mock's
      // prepareExternalUrl ignores APP_BASE_HREF, which is the thing under
      // test here.
      { provide: LocationStrategy, useClass: PathLocationStrategy },
      { provide: APP_BASE_HREF, useValue: baseHref },
      { provide: CONFIG_LOADER, useValue: async () => ({ ...EMPTY_CONFIGURATION, ...config }) },
      {
        provide: AUTHN_CLIENT,
        useValue: {
          getUserInfo: () => ({
            subscribe: (observer: { next(value: unknown): void; error(err: unknown): void }) => {
              if (outcome === 'unauthenticated') {
                observer.error(new Error('unauthenticated'));
              } else {
                observer.next(outcome);
              }
            },
          }),
        },
      },
    ],
  });

  await TestBed.inject(ConfigService).loadConfig();

  const redirect = vi
    .spyOn(TestBed.inject(SessionService), 'redirectToLogin')
    .mockImplementation(() => {});

  const state = { url: '/manage' } as RouterStateSnapshot;
  const allowed = await TestBed.runInInjectionContext(() =>
    authGuard({} as ActivatedRouteSnapshot, state),
  );

  return {
    allowed: allowed === true,
    redirectedTo: redirect.mock.calls[0]?.[0],
  };
}

describe('authGuard', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('lets a signed-in visitor through', async () => {
    const result = await runGuard(
      { authnApiUrl: AUTHN_API_URL },
      { user: { organizationIds: ['org'] } },
    );

    expect(result.allowed).toBe(true);
    expect(result.redirectedTo).toBeUndefined();
  });

  it('sends a visitor without a session to the login, with the route to come back to', async () => {
    const result = await runGuard({ authnApiUrl: AUTHN_API_URL }, 'unauthenticated');

    expect(result.allowed).toBe(false);
    expect(result.redirectedTo).toBe('/manage');
  });

  // A session whose token carries no user is no session at all.
  it('sends a visitor with an empty session to the login', async () => {
    const result = await runGuard({ authnApiUrl: AUTHN_API_URL }, { user: undefined });

    expect(result.allowed).toBe(false);
    expect(result.redirectedTo).toBe('/manage');
  });

  // The demo bundle answers the portal from fixtures and has nothing to sign
  // in against, so the guard must not send it anywhere.
  it('lets the navigation through when no authn URL is configured', async () => {
    const result = await runGuard({}, 'unauthenticated');

    expect(result.allowed).toBe(true);
    expect(result.redirectedTo).toBeUndefined();
  });

  // Coming back from the login still without a session means sending the
  // visitor again would do the same thing. Letting the navigation through
  // surfaces the API's own error instead of looping the tab.
  it('does not send a visitor to the login twice', async () => {
    sessionStorage.setItem('marketplace_login_attempted', '1');

    const result = await runGuard({ authnApiUrl: AUTHN_API_URL }, 'unauthenticated');

    expect(result.allowed).toBe(true);
    expect(result.redirectedTo).toBeUndefined();
  });

  // A session that resolves clears the flag, so a later expiry can send the
  // visitor to the login again rather than being written off as a loop.
  it('clears the earlier attempt once a session resolves', async () => {
    sessionStorage.setItem('marketplace_login_attempted', '1');

    await runGuard({ authnApiUrl: AUTHN_API_URL }, { user: { organizationIds: ['org'] } });

    expect(sessionStorage.getItem('marketplace_login_attempted')).toBeNull();
  });

  // The router URL carries no base href, so a portal served under a subpath
  // has to have it put back or the visitor returns to the wrong page.
  it('returns to a route under the deployment base href', async () => {
    const result = await runGuard({ authnApiUrl: AUTHN_API_URL }, 'unauthenticated', '/portal/');

    expect(result.redirectedTo).toBe('/portal/manage');
  });
});

describe('SessionService.loginUrl', () => {
  async function loginUrl(config: Partial<AppConfiguration>, path: string): Promise<string> {
    TestBed.configureTestingModule({
      providers: [
        {
          provide: CONFIG_LOADER,
          useValue: async () => ({ ...EMPTY_CONFIGURATION, ...config }),
        },
        { provide: AUTHN_CLIENT, useValue: {} },
      ],
    });
    await TestBed.inject(ConfigService).loadConfig();

    return TestBed.inject(SessionService).loginUrl(path);
  }

  // The console owns the only password form; it resolves where to come back
  // to from the app name against its own configuration, so no URL of ours
  // needs to be trusted on its side.
  it('hands off to the console when there is one', async () => {
    const url = await loginUrl({ authnApiUrl: AUTHN_API_URL, consoleUrl: CONSOLE_URL }, '/manage');

    expect(url).toBe(`${CONSOLE_URL}/login?app=marketplace-registry&path=%2Fmanage`);
  });

  // A console URL with a path of its own still names the login at its root.
  it('puts the login at the console root', async () => {
    const url = await loginUrl(
      { authnApiUrl: AUTHN_API_URL, consoleUrl: `${CONSOLE_URL}/organizations/acme` },
      '/manage',
    );

    expect(url).toBe(`${CONSOLE_URL}/login?app=marketplace-registry&path=%2Fmanage`);
  });

  // No console deployed here, so the OIDC login answers instead and needs an
  // absolute return_to.
  it('falls back to authn with the return URL encoded', async () => {
    const url = await loginUrl({ authnApiUrl: `${AUTHN_API_URL}/` }, '/manage');

    // The trailing slash on the configured URL is not doubled up.
    expect(url).toBe(
      `${AUTHN_API_URL}/login?return_to=${encodeURIComponent(`${window.location.origin}/manage`)}`,
    );
  });
});
