// Demo-only stand-in for AuthnApiService. Seeds a stub user so the auth guard and the
// root shell see an authenticated session with no login and no backend.
import { BehaviorSubject, Observable } from 'rxjs';
import type { User } from '../../generated/authn/v1/authn_pb';
import { demoUser } from './fixtures';

export class FakeAuthnApiService {
  private currentUserSubject = new BehaviorSubject<User | undefined>(demoUser);

  currentUser$: Observable<User | undefined> = this.currentUserSubject.asObservable();

  async login(): Promise<void> {
    this.currentUserSubject.next(demoUser);
  }

  async getUserInfo(): Promise<User | undefined> {
    return this.currentUserSubject.value;
  }

  async initializeAuth(): Promise<void> {
    // Already authenticated via the seeded subject; nothing to do.
  }

  async refreshToken(): Promise<void> {
    // No-op in the demo build.
  }

  async logout(): Promise<void> {
    this.currentUserSubject.next(undefined);
  }

  isAuthenticated(): boolean {
    return this.currentUserSubject.value !== undefined;
  }
}
