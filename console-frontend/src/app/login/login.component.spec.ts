import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import LoginComponent from './login.component';
import AuthnApiService from '../authn-api.service';
import { ConfigService, type AppConfiguration } from '../config.service';
import { TitleService } from '../title.service';

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
 * `initializeAuth` settles, and then it is `signedIn`.
 */
async function openLogin(signedIn: boolean) {
  let authenticated = false;
  const initializeAuth = vi.fn(async () => {
    authenticated = signedIn;
  });
  const navigateByUrl = vi.fn();
  await TestBed.configureTestingModule({
    imports: [LoginComponent],
    providers: [
      {
        provide: AuthnApiService,
        useValue: { isAuthenticated: () => authenticated, initializeAuth },
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

  return {
    login: TestBed.createComponent(LoginComponent).componentInstance,
    initializeAuth,
    navigateByUrl,
  };
}

describe('LoginComponent', () => {
  afterEach(() => vi.restoreAllMocks());

  it('sends a visitor who is already signed in straight back to the portal', async () => {
    const assign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
    const { login, initializeAuth, navigateByUrl } = await openLogin(true);

    await login.ngOnInit();

    // The portal's session is the parent-domain cookie, which this origin's
    // auth hint need not know about, so the check must not stop at the hint.
    expect(initializeAuth).toHaveBeenCalledWith({ ignoreHint: true });
    expect(assign).toHaveBeenCalledWith(`${DEVELOPER_URL}/manage`);
    expect(navigateByUrl).not.toHaveBeenCalled();
  });

  it('stays on the form for a visitor with no session', async () => {
    const assign = vi.spyOn(window.location, 'assign').mockImplementation(() => {});
    const { login, navigateByUrl } = await openLogin(false);

    await login.ngOnInit();

    expect(assign).not.toHaveBeenCalled();
    expect(navigateByUrl).not.toHaveBeenCalled();
  });
});
