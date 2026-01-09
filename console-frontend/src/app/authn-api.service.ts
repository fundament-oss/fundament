import { Injectable } from '@angular/core';
import { BehaviorSubject, Observable } from 'rxjs';
import type { User, GetUserInfoResponse } from '../generated/authn/v1/authn_pb';

const CONFIG = {
  apiBaseUrl: 'http://authn.127.0.0.1.nip.io:8080',
  servicePath: '/authn.v1.AuthnService',
};

@Injectable({
  providedIn: 'root',
})
export class AuthnApiService {
  private currentUserSubject = new BehaviorSubject<User | undefined>(undefined);
  public currentUser$: Observable<User | undefined> = this.currentUserSubject.asObservable();

  private async connectRpc<T>(method: string, request: object = {}): Promise<T> {
    const url = `${CONFIG.apiBaseUrl}${CONFIG.servicePath}/${method}`;

    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'include', // Important: send cookies with requests
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: response.statusText }));
      throw new Error(error.message || `Request failed: ${response.status}`);
    }

    return response.json();
  }

  getLoginUrl(): string {
    return `${CONFIG.apiBaseUrl}/login`;
  }

  async login(email: string, password: string): Promise<void> {
    // Submit credentials to Dex local connector endpoint
    const returnUrl = `${window.location.origin}/`;

    const response = await fetch(`${CONFIG.apiBaseUrl}/login/password`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      credentials: 'include', // Important: allow cookies to be set
      body: JSON.stringify({
        email,
        password,
        return_to: returnUrl,
      }),
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: response.statusText }));
      throw new Error(error.message || `Login failed: ${response.status}`);
    }

    // Backend sets HTTP-only cookie, no need to handle token in frontend
    // Set hint flag (optimization to avoid unnecessary 401s on page load)
    localStorage.setItem('auth_hint', 'true');

    // Fetch user info immediately after successful login to populate state
    await this.getUserInfo();
  }

  async getUserInfo(): Promise<User | undefined> {
    const response = await this.connectRpc<GetUserInfoResponse>('GetUserInfo', {});
    this.currentUserSubject.next(response.user);
    return response.user;
  }

  async initializeAuth(): Promise<void> {
    // Check hint flag to avoid unnecessary API calls when we know user isn't logged in
    // This is just an optimization - the server (via HTTP-only cookie) is still the source of truth
    if (!this.hasAuthHint()) {
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
    const response = await fetch(`${CONFIG.apiBaseUrl}/refresh`, {
      method: 'POST',
      credentials: 'include',
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: response.statusText }));
      throw new Error(error.message || `Request failed: ${response.status}`);
    }
  }

  async logout(): Promise<void> {
    const response = await fetch(`${CONFIG.apiBaseUrl}/logout`, {
      method: 'POST',
      credentials: 'include',
    });

    if (!response.ok) {
      throw new Error(`Logout failed: ${response.status}`);
    }

    // Clear user state and hint
    this.currentUserSubject.next(undefined);
    localStorage.removeItem('auth_hint');
  }

  isAuthenticated(): boolean {
    // Check if we have a current user in our state
    return this.currentUserSubject.value !== undefined;
  }

  private hasAuthHint(): boolean {
    // This is just an optimization hint, not the source of truth
    return localStorage.getItem('auth_hint') === 'true';
  }
}
