// Demo-only stand-in for AuthnApiService. Seeds a stub user so the auth guard and the
// root shell see an authenticated session with no login and no backend.
import { BehaviorSubject, Observable } from 'rxjs';
import type { User } from '../../generated/authn/v1/authn_pb';
import type AuthnApiService from '../authn-api.service';
import type { LoginMethodsResponse } from '../../generated/authn-api';
import { demoUser } from './fixtures';

export default class FakeAuthnApiService implements Pick<
  AuthnApiService,
  | 'currentUser$'
  | 'login'
  | 'getLoginMethods'
  | 'getUserInfo'
  | 'initializeAuth'
  | 'refreshToken'
  | 'logout'
  | 'isAuthenticated'
> {
  private currentUserSubject = new BehaviorSubject<User | undefined>(demoUser);

  currentUser$: Observable<User | undefined> = this.currentUserSubject.asObservable();

  async login(): Promise<void> {
    this.currentUserSubject.next(demoUser);
  }

  async getUserInfo(): Promise<User | undefined> {
    return this.currentUserSubject.value;
  }

  // These must stay instance methods to match AuthnApiService's shape. The first two
  // are genuinely no-ops here: the subject is seeded at construction and never expires.
  /* eslint-disable class-methods-use-this */
  async initializeAuth(): Promise<void> {
    // Already authenticated via the seeded subject; nothing to do.
  }

  async refreshToken(): Promise<void> {
    // No-op in the demo build.
  }

  // There is no identity provider to send a demo visitor to.
  async getLoginMethods(): Promise<LoginMethodsResponse> {
    return { password: true };
  }
  /* eslint-enable class-methods-use-this */

  async logout(): Promise<void> {
    this.currentUserSubject.next(undefined);
  }

  isAuthenticated(): boolean {
    return this.currentUserSubject.value !== undefined;
  }
}
