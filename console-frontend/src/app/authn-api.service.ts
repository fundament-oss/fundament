import { Injectable, inject } from '@angular/core';
import { BehaviorSubject, Observable, firstValueFrom } from 'rxjs';
import type { User } from '../generated/authn/v1/authn_pb';
import { AUTHN } from '../connect/tokens';
import { ConfigService } from './config.service';
import { client as authnRestClient } from '../generated/authn-api/client.gen';
import {
  handleLoginMethods,
  handlePasswordLogin,
  handleRefresh,
  handleLogout,
  type LoginMethodsResponse,
} from '../generated/authn-api';
import OrganizationContextService from './organization-context.service';

const COOKIE_REQUEST_TIMEOUT_MS = 10_000;

@Injectable({
  providedIn: 'root',
})
export default class AuthnApiService {
  private client = inject(AUTHN);

  private restClient = authnRestClient;

  private configService = inject(ConfigService);

  private organizationContext = inject(OrganizationContextService);

  private currentUserSubject = new BehaviorSubject<User | undefined>(undefined);

  public currentUser$: Observable<User | undefined> = this.currentUserSubject.asObservable();

  private pendingGetUserInfo: Promise<User | undefined> | null = null;

  // Login, refresh and logout all answer by setting the auth cookie, and the
  // browser keeps whichever answer lands last. A refresh answered after a
  // logout would put the session back, or after a login the session before it,
  // so they go out one at a time.
  private lastCookieRequest: Promise<unknown> = Promise.resolve();

  constructor() {
    // Configure the authn REST client with the runtime base URL and credentials
    this.restClient.setConfig({
      baseUrl: this.configService.getConfig().authnApiUrl,
      credentials: 'include',
    });
  }

  // No `return_to`: password login answers with the token as JSON and sets the
  // cookie, it never redirects, so the caller stays on the page and routes
  // itself. Only the OIDC `/login` takes one.
  async login(email: string, password: string): Promise<void> {
    const { error } = await this.oneAtATime((signal) =>
      handlePasswordLogin({
        client: this.restClient,
        body: { email, password },
        signal,
      }),
    );

    if (error) {
      throw new Error(error.error || 'Login failed');
    }

    // Backend sets HTTP-only cookie, no need to handle token in frontend
    // Set hint flag (optimization to avoid unnecessary 401s on page load)
    localStorage.setItem('auth_hint', 'true');

    // Fetch user info immediately after successful login to populate state
    await this.getUserInfo();
  }

  /**
   * How this deployment signs people in: the password form, a button to the
   * identity provider, or both. When authn-api does not answer, the form: it
   * is what every deployment had before there was a choice.
   */
  async getLoginMethods(): Promise<LoginMethodsResponse> {
    try {
      const { data } = await handleLoginMethods({ client: this.restClient });
      return data ?? { password: true };
    } catch {
      return { password: true };
    }
  }

  /**
   * Where a login at the identity provider starts. Unlike the password login
   * this is a page load: the provider's own pages come in between, and
   * authn-api sends the visitor to `returnTo` once the cookie is set.
   */
  ssoLoginUrl(returnTo: string): string {
    const query = new URLSearchParams({ return_to: returnTo });
    return `${this.configService.getConfig().authnApiUrl}/login?${query}`;
  }

  /**
   * Called on the way out to an external provider. The page that loads on the
   * way back only asks authn-api for the session when the hint says there may
   * be one, and nothing else sets it for this kind of login. If the login is
   * abandoned instead, the next load asks once and clears it again.
   */
  // An instance method so the login page can call it on the injected (and in
  // tests, faked) service.
  // eslint-disable-next-line class-methods-use-this
  expectSessionAfterRedirect(): void {
    try {
      localStorage.setItem('auth_hint', 'true');
    } catch {
      // Without the hint the visitor lands on the form and logs in again.
    }
  }

  async getUserInfo(): Promise<User | undefined> {
    if (this.pendingGetUserInfo) {
      return this.pendingGetUserInfo;
    }
    this.pendingGetUserInfo = firstValueFrom(this.client.getUserInfo({}))
      .then((response) => {
        this.currentUserSubject.next(response.user);
        return response.user;
      })
      .finally(() => {
        this.pendingGetUserInfo = null;
      });
    return this.pendingGetUserInfo;
  }

  /**
   * `ignoreHint` asks authn-api even without the hint. The hint lives in this
   * origin's storage while the cookie lives on the parent domain, so a session
   * can exist that the hint does not know about.
   */
  async initializeAuth({ ignoreHint = false }: { ignoreHint?: boolean } = {}): Promise<void> {
    // Check hint flag to avoid unnecessary API calls when we know user isn't logged in
    // This is just an optimization - the server (via HTTP-only cookie) is still the source of truth
    if (!ignoreHint && !AuthnApiService.hasAuthHint()) {
      this.currentUserSubject.next(undefined);
      return;
    }

    // getUserInfo() already deduplicates concurrent calls; just attach the init-specific error handler
    await this.getUserInfo().then(
      () => {
        // Only an optimization, so a full or blocked storage must not turn a
        // session that exists into a failed check.
        try {
          localStorage.setItem('auth_hint', 'true');
        } catch {
          // Without the hint the next load skips the check; logging in sets it.
        }
      },
      () => {
        // Not authenticated or session expired - clear the hint
        this.currentUserSubject.next(undefined);
        localStorage.removeItem('auth_hint');
      },
    );
  }

  async refreshToken(): Promise<void> {
    const { error } = await this.oneAtATime((signal) =>
      handleRefresh({ client: this.restClient, signal }),
    );

    if (error) {
      throw new Error(error.error || 'Refresh failed');
    }
  }

  async logout(): Promise<void> {
    const { error } = await this.oneAtATime((signal) =>
      handleLogout({ client: this.restClient, signal }),
    );

    if (error) {
      throw new Error(error.error || 'Logout failed');
    }

    // Clear user state, hint, and organization selection
    this.currentUserSubject.next(undefined);
    localStorage.removeItem('auth_hint');
    this.organizationContext.clearOrganizationId();
  }

  isAuthenticated(): boolean {
    // Check if we have a current user in our state
    return this.currentUserSubject.value !== undefined;
  }

  // A request that never answers would hold up every one queued behind it, a
  // logout included, so each is aborted after a while. Aborted rather than
  // given up on: the browser then drops its answer, where one that still
  // arrived could set the cookie after the requests that followed it.
  private oneAtATime<T>(send: (signal: AbortSignal) => Promise<T>): Promise<T> {
    const request = this.lastCookieRequest.then(() => {
      const abort = new AbortController();
      const timer = setTimeout(() => abort.abort(), COOKIE_REQUEST_TIMEOUT_MS);
      return send(abort.signal).finally(() => clearTimeout(timer));
    });
    this.lastCookieRequest = request.then(
      () => undefined,
      () => undefined,
    );
    return request;
  }

  private static hasAuthHint(): boolean {
    // This is just an optimization hint, not the source of truth
    return localStorage.getItem('auth_hint') === 'true';
  }
}
