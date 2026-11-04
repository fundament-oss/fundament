import { ComponentFixture, TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import LoginComponent from './login.component';
import AuthnApiService from '../authn-api.service';
import { ConfigService, type AppConfiguration } from '../config.service';
import { TitleService } from '../title.service';
import type { LoginMethodsResponse } from '../../generated/authn-api';

const DEVELOPER_URL = 'https://marketplace-registry.fundament.localhost:8443';

const config: AppConfiguration = {
  authnApiUrl: '',
  organizationApiUrl: '',
  marketplaceApiUrl: '',
  kubeApiProxyUrl: '',
  pluginProxyUrl: '',
  developerUrl: DEVELOPER_URL,
};

/**
 * The login page as the developer portal opens it, on a fresh load: the shell
 * has not heard back from authn-api yet, so the session is unknown until
 * `initializeAuth` settles, and then it is `signedIn`. `methods` is how
 * authn-api says this deployment signs people in.
 */
async function openLogin(signedIn: boolean, methods: LoginMethodsResponse = { password: true }) {
  let authenticated = false;
  const initializeAuth = vi.fn(async () => {
    authenticated = signedIn;
  });
  const getLoginMethods = vi.fn(async () => methods);
  const expectSessionAfterRedirect = vi.fn();
  const navigateByUrl = vi.fn();
  await TestBed.configureTestingModule({
    imports: [LoginComponent],
    providers: [
      {
        provide: AuthnApiService,
        useValue: {
          isAuthenticated: () => authenticated,
          initializeAuth,
          getLoginMethods,
          expectSessionAfterRedirect,
          ssoLoginUrl: (returnTo: string) =>
            `https://authn.test/login?${new URLSearchParams({ return_to: returnTo })}`,
        },
      },
      { provide: ConfigService, useValue: { getConfig: () => config } },
      { provide: TitleService, useValue: { setTitle: () => {}, setDescription: () => {} } },
      { provide: Router, useValue: { navigateByUrl } },
      {
        provide: ActivatedRoute,
        useValue: {
          snapshot: {
            queryParamMap: convertToParamMap({ app: 'marketplace-registry', path: '/manage' }),
          },
        },
      },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(LoginComponent);
  return {
    fixture,
    login: fixture.componentInstance,
    initializeAuth,
    getLoginMethods,
    expectSessionAfterRedirect,
    navigateByUrl,
  };
}

/**
 * The page as rendered once `ngOnInit` has loaded the login methods. The first
 * `detectChanges` runs `ngOnInit`; nothing Angular tracks waits for the
 * promise it returns, so the wait is for what it sets.
 */
async function render(fixture: ComponentFixture<LoginComponent>) {
  fixture.detectChanges();
  await vi.waitFor(() => expect(fixture.componentInstance.methods()).not.toBeNull());
  fixture.detectChanges();
  const page = fixture.nativeElement as HTMLElement;
  return {
    ssoButtons: Array.from(page.querySelectorAll('nldd-button[href]')) as HTMLElement[],
    form: page.querySelector('nldd-form'),
    divider: page.querySelector('nldd-divider'),
    note: page.querySelector('.sso-note'),
  };
}

describe('LoginComponent', () => {
  afterEach(() => vi.restoreAllMocks());

  // Also how an SSO login ends: authn-api sends the visitor back to this page
  // with the cookie set, and the page sends them on.
  it('sends a visitor who is already signed in straight back to the portal', async () => {
    const assign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
    const { login, initializeAuth, getLoginMethods, navigateByUrl } = await openLogin(true, {
      password: false,
      provider_name: 'Pleio',
    });

    await login.ngOnInit();

    // The portal's session is the parent-domain cookie, which this origin's
    // auth hint need not know about, so the check must not stop at the hint.
    expect(initializeAuth).toHaveBeenCalledWith({ ignoreHint: true });
    expect(assign).toHaveBeenCalledWith(`${DEVELOPER_URL}/manage`);
    expect(navigateByUrl).not.toHaveBeenCalled();
    expect(getLoginMethods).not.toHaveBeenCalled();
  });

  it('stays on the form for a visitor with no session', async () => {
    const assign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
    const { login, navigateByUrl } = await openLogin(false);

    await login.ngOnInit();

    expect(assign).not.toHaveBeenCalled();
    expect(navigateByUrl).not.toHaveBeenCalled();
  });

  it('offers only the provider when there is no password login', async () => {
    const { fixture } = await openLogin(false, { password: false, provider_name: 'Pleio' });

    const { ssoButtons, form, divider } = await render(fixture);

    expect(ssoButtons).toHaveLength(1);
    expect(ssoButtons[0].getAttribute('text')).toBe('Log in with Pleio');
    expect(ssoButtons[0].getAttribute('appearance')).toBe('primary');
    // The page as it was opened, hand-off parameters and all, so the visitor
    // still ends up at the portal.
    const url = new URL(ssoButtons[0].getAttribute('href')!);
    expect(url.searchParams.get('return_to')).toBe(window.location.href);
    expect(form).toBeNull();
    expect(divider).toBeNull();
  });

  it('says under the Pleio button that it is a fieldlab login', async () => {
    const { fixture } = await openLogin(false, { password: false, provider_name: 'Pleio' });

    const { note } = await render(fixture);

    expect(note?.textContent).toContain('fieldlab');
  });

  it('says nothing under the button of any other provider', async () => {
    const { fixture } = await openLogin(false, { password: true, provider_name: 'Dex' });

    const { note } = await render(fixture);

    expect(note).toBeNull();
  });

  it('shows only the form where Dex is the identity provider', async () => {
    const { fixture, getLoginMethods } = await openLogin(false, { password: true });

    const { ssoButtons, form, divider } = await render(fixture);

    expect(getLoginMethods).toHaveBeenCalledTimes(1);
    expect(ssoButtons).toHaveLength(0);
    expect(form).not.toBeNull();
    expect(divider).toBeNull();
  });

  it('separates the provider from the form when there are both', async () => {
    const { fixture } = await openLogin(false, { password: true, provider_name: 'Dex' });

    const { ssoButtons, form, divider } = await render(fixture);

    expect(ssoButtons[0].getAttribute('appearance')).toBe('secondary');
    expect(form).not.toBeNull();
    expect(divider).not.toBeNull();
  });

  // Without the hint, the page the provider sends the visitor back to would
  // not ask for the session and show the login again.
  it('expects a session on the way out to the identity provider', async () => {
    const { fixture, expectSessionAfterRedirect } = await openLogin(false, {
      password: false,
      provider_name: 'Pleio',
    });
    const {
      ssoButtons: [button],
    } = await render(fixture);

    button.dispatchEvent(new MouseEvent('click', { cancelable: true }));

    expect(expectSessionAfterRedirect).toHaveBeenCalled();
  });
});
