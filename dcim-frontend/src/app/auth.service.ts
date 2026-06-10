import { Injectable, inject, signal, computed } from '@angular/core';
import { ConfigService } from './config.service';

export interface DcimUser {
  id: string;
  name: string;
}

@Injectable({
  providedIn: 'root',
})
export default class AuthService {
  private configService = inject(ConfigService);

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
    this.pendingUserInfo = fetch(`${this.authnApiUrl}/userinfo`, {
      credentials: 'include',
    })
      .then(async (resp) => {
        if (!resp.ok) throw new Error('unauthenticated');
        const user = (await resp.json()) as DcimUser;
        this.currentUser.set(user);
        return user;
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
    await this.getUserInfo().catch(() => {
      this.currentUser.set(undefined);
      localStorage.removeItem('dcim_auth_hint');
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
