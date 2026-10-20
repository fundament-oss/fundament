import { Injectable, inject, signal, computed } from '@angular/core';
import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { ConfigService } from './config.service';
import { AuthnService } from '../generated/dcimauthn/v1/dcimauthn_pb';
import { AUTHN_TRANSPORT } from '../connect/tokens';

export interface DcimUser {
  id: string;
  name: string;
}

@Injectable({
  providedIn: 'root',
})
export default class AuthService {
  private configService = inject(ConfigService);

  private authnClient = createClient(AuthnService, inject(AUTHN_TRANSPORT));

  private currentUser = signal<DcimUser | undefined>(undefined);

  readonly isAuthenticated = computed(() => this.currentUser() !== undefined);

  readonly user = this.currentUser.asReadonly();

  private pendingUserInfo: Promise<DcimUser | undefined> | null = null;

  private get authnApiUrl(): string {
    return this.configService.getConfig().authnApiUrl;
  }

  async login(email: string, password: string): Promise<void> {
    const response = await fetch(`${this.authnApiUrl}/login/password`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });

    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new Error((body as { error?: string }).error ?? 'Login failed');
    }

    localStorage.setItem('dcim_auth_hint', 'true');
    await this.getUserInfo();
  }

  async getUserInfo(): Promise<DcimUser | undefined> {
    if (this.pendingUserInfo) {
      return this.pendingUserInfo;
    }
    this.pendingUserInfo = this.authnClient
      .getUserInfo({})
      .then((resp) => {
        const user: DcimUser = { id: resp.user?.id ?? '', name: resp.user?.name ?? '' };
        this.currentUser.set(user);
        return user;
      })
      .catch((err: unknown) => {
        // Only Unauthenticated means the session is over; any other failure
        // (the service or its database briefly unavailable) must not end it.
        if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
          throw new Error('unauthenticated');
        }
        throw new Error('userinfo unavailable');
      })
      .finally(() => {
        this.pendingUserInfo = null;
      });
    return this.pendingUserInfo;
  }

  async initializeAuth(): Promise<void> {
    if (!AuthService.hasAuthHint()) {
      this.currentUser.set(undefined);
      return;
    }
    await this.getUserInfo().catch((err: unknown) => {
      this.currentUser.set(undefined);
      // Keep the hint on a transient failure: the cookie may still be
      // valid, and the next navigation retries.
      if (err instanceof Error && err.message === 'unauthenticated') {
        localStorage.removeItem('dcim_auth_hint');
      }
    });
  }

  async refreshToken(): Promise<void> {
    const response = await fetch(`${this.authnApiUrl}/refresh`, {
      method: 'POST',
      credentials: 'include',
    });
    if (!response.ok) throw new Error('Refresh failed');
  }

  async logout(): Promise<void> {
    await fetch(`${this.authnApiUrl}/logout`, {
      method: 'POST',
      credentials: 'include',
    });
    this.currentUser.set(undefined);
    localStorage.removeItem('dcim_auth_hint');
  }

  private static hasAuthHint(): boolean {
    return localStorage.getItem('dcim_auth_hint') === 'true';
  }
}
