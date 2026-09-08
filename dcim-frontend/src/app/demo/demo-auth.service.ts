import { Injectable, computed, signal } from '@angular/core';
import { DcimUser } from '../auth.service';
import { currentUser } from './fixtures';

/** Already signed in, as whoever the fixtures say you are: the demo is there to
 *  show the screens, and a login form with no backend behind it shows nothing. */
@Injectable()
export default class DemoAuthService {
  private readonly signedIn = signal<DcimUser | undefined>({
    id: currentUser.id,
    name: currentUser.name,
  });

  readonly isAuthenticated = computed(() => this.signedIn() !== undefined);

  readonly user = this.signedIn.asReadonly();

  async login(): Promise<void> {
    this.signedIn.set({ id: currentUser.id, name: currentUser.name });
  }

  async getUserInfo(): Promise<DcimUser | undefined> {
    return this.signedIn();
  }

  /** Nothing to initialize and no token to refresh, but the guard calls both. */
  async initializeAuth(): Promise<void> {
    this.signedIn.set({ id: currentUser.id, name: currentUser.name });
  }

  async refreshToken(): Promise<void> {
    this.signedIn.set({ id: currentUser.id, name: currentUser.name });
  }

  async logout(): Promise<void> {
    this.signedIn.set(undefined);
  }
}
