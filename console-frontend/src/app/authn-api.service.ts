import { Injectable, inject } from '@angular/core';
import { BehaviorSubject, Observable, firstValueFrom } from 'rxjs';
import type { User } from '../generated/authn/v1/authn_pb';
import { AUTHN } from '../connect/tokens';
import { environment } from '../environments/environment';

@Injectable({
  providedIn: 'root',
})
export class AuthnApiService {
  private client = inject(AUTHN);
  private currentUserSubject = new BehaviorSubject<User | undefined>(undefined);
  public currentUser$: Observable<User | undefined> = this.currentUserSubject.asObservable();

  async login(email: string, password: string): Promise<void> {
    // Submit credentials to Dex local connector endpoint
    const returnUrl = `${window.location.origin}/`;

    const response = await fetch(`${environment.authnApiUrl}/login/password`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      credentials: 'include', // Allow cookies to be set
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
    const response = await firstValueFrom(this.client.getUserInfo({}));
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
    const response = await fetch(`${environment.authnApiUrl}/refresh`, {
      method: 'POST',
      credentials: 'include',
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: response.statusText }));
      throw new Error(error.message || `Request failed: ${response.status}`);
    }
  }

  async logout(): Promise<void> {
    const response = await fetch(`${environment.authnApiUrl}/logout`, {
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
