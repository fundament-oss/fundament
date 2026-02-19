import { Injectable, inject } from '@angular/core';
import { BehaviorSubject, Observable, firstValueFrom } from 'rxjs';
import type { User } from '../generated/authn/v1/authn_pb';
import { AUTHN } from '../connect/tokens';
import { ConfigService } from './config.service';
import { client as authnRestClient } from '../generated/authn-api/client.gen';
import { handlePasswordLogin, handleRefresh, handleLogout } from '../generated/authn-api';
import OrganizationContextService from './organization-context.service';

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

  constructor() {
    // Configure the authn REST client with the runtime base URL and credentials
    this.restClient.setConfig({
      baseUrl: this.configService.getConfig().authnApiUrl,
      credentials: 'include',
    });
  }

  async login(email: string, password: string): Promise<void> {
    const returnUrl = `${window.location.origin}/`;

    const { error } = await handlePasswordLogin({
      client: this.restClient,
      body: {
        email,
        password,
        return_to: returnUrl,
      },
    });

    if (error) {
      throw new Error(error.error || 'Login failed');
    }

    // Backend sets HTTP-only cookie, no need to handle token in frontend
    // Set hint flag (optimization to avoid unnecessary 401s on page load)
    localStorage.setItem('auth_hint', 'true');

    // Fetch user info immediately after successful login to populate state
    await this.getUserInfo();
  }

  async getUserInfo(): Promise<User | undefined> {
    const response = await firstValueFrom(this.client.getUserInfo({}));
    this.currentUserSubject.next(response.user);
    return response.user;
  }

  async initializeAuth(): Promise<void> {
    // Check hint flag to avoid unnecessary API calls when we know user isn't logged in
    // This is just an optimization - the server (via HTTP-only cookie) is still the source of truth
    if (!AuthnApiService.hasAuthHint()) {
      this.currentUserSubject.next(undefined);
      return;
    }

    try {
      // Try to fetch user info - if successful, user is authenticated via HTTP-only cookie
      const userInfo = await this.getUserInfo();
      this.currentUserSubject.next(userInfo);
    } catch {
      // Not authenticated or session expired - clear the hint
      this.currentUserSubject.next(undefined);
      localStorage.removeItem('auth_hint');
    }
  }

  async refreshToken(): Promise<void> {
    const { error } = await handleRefresh({ client: this.restClient });

    if (error) {
      throw new Error(error.error || 'Refresh failed');
    }
  }

  async logout(): Promise<void> {
    const { error } = await handleLogout({ client: this.restClient });

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

  private static hasAuthHint(): boolean {
    // This is just an optimization hint, not the source of truth
    return localStorage.getItem('auth_hint') === 'true';
  }
}
