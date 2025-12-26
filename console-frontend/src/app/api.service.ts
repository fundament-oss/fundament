import { Injectable } from '@angular/core';
import { BehaviorSubject, Observable } from 'rxjs';

const CONFIG = {
  apiBaseUrl: 'http://authn.127.0.0.1.nip.io:8080',
  servicePath: '/authn.v1.AuthnService',
};

export interface UserInfo {
  id: string;
  tenantId: string;
  name: string;
  externalId: string;
  groups: string[];
}

export interface UserResponse {
  user: UserInfo;
}

@Injectable({
  providedIn: 'root',
})
export class ApiService {
  private currentUserSubject = new BehaviorSubject<UserInfo | null>(null);
  public currentUser$: Observable<UserInfo | null> = this.currentUserSubject.asObservable();
  private accessToken: string | null = null;

  private async connectRpc<T>(method: string, request: object = {}): Promise<T> {
    const url = `${CONFIG.apiBaseUrl}${CONFIG.servicePath}/${method}`;

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    // Add Authorization header if we have a token
    if (this.accessToken) {
      headers['Authorization'] = `Bearer ${this.accessToken}`;
    }

    const response = await fetch(url, {
      method: 'POST',
      headers,
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

    // Extract and store the JWT token from the response
    const loginData = await response.json();
    if (loginData.access_token) {
      this.accessToken = loginData.access_token;
      localStorage.setItem('access_token', loginData.access_token);
    }

    // Set hint flag (optimization to avoid unnecessary 401s on page load)
    localStorage.setItem('auth_hint', 'true');

    // Fetch user info immediately after successful login to populate state
    await this.getUserInfo();
  }

  async getUserInfo(): Promise<UserInfo> {
    const response = await this.connectRpc<UserResponse>('GetUserInfo', {});
    this.currentUserSubject.next(response.user);
    return response.user;
  }

  async initializeAuth(): Promise<void> {
    // Restore token from localStorage if available
    const storedToken = localStorage.getItem('access_token');
    if (storedToken) {
      this.accessToken = storedToken;
    }

    // Check hint flag to avoid unnecessary API calls when we know user isn't logged in
    // This is just an optimization - the server (via HTTP-only cookie) is still the source of truth
    if (!this.hasAuthHint()) {
      this.currentUserSubject.next(null);
      return;
    }

    try {
      // Try to fetch user info - if successful, user is authenticated
      const userInfo = await this.getUserInfo();
      this.currentUserSubject.next(userInfo);
    } catch {
      // Not authenticated or session expired - clear the hint and token
      this.currentUserSubject.next(null);
      this.accessToken = null;
      localStorage.removeItem('auth_hint');
      localStorage.removeItem('access_token');
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

    // Clear user state, token, and hint
    this.currentUserSubject.next(null);
    this.accessToken = null;
    localStorage.removeItem('auth_hint');
    localStorage.removeItem('access_token');
  }

  isAuthenticated(): boolean {
    // Check if we have a current user in our state
    return this.currentUserSubject.value !== null;
  }

  getAccessToken(): string | null {
    return this.accessToken;
  }

  private hasAuthHint(): boolean {
    // This is just an optimization hint, not the source of truth
    return localStorage.getItem('auth_hint') === 'true';
  }
}
