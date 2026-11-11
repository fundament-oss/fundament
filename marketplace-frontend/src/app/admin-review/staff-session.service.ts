import { Injectable, inject, signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';
import { DCIM_AUTHN_CLIENT } from '../../connect/dcim-authn';
import { ConfigService } from '../config.service';

export interface StaffUser {
  id: string;
  name: string;
}

/**
 * The reviewer's session in the review backoffice (FUN-20).
 *
 * Reviewers are Fundament staff, so marketplace-admin-api accepts the DCIM
 * token (`dcim_auth`) and refuses a console one. dcim-authn-api sets that
 * cookie on the parent domain, which is why signing in to DCIM used to be the
 * only way in: the backoffice had no login of its own and simply found the
 * cookie DCIM had left. Signing in here posts to the same endpoint the DCIM
 * login does, so there is still one staff session, shared by both.
 *
 * There is deliberately no refresh attempt when `GetUserInfo` says no:
 * `/refresh` validates the same cookie, so it can only succeed where
 * `GetUserInfo` already did.
 */
@Injectable({ providedIn: 'root' })
export default class StaffSessionService {
  private readonly client = inject(DCIM_AUTHN_CLIENT);

  private readonly configService = inject(ConfigService);

  /** The signed-in reviewer, or null when there is none (or not yet known). */
  readonly user = signal<StaffUser | null>(null);

  private pending?: Promise<StaffUser | null>;

  // Bumped by every lookup and by every sign-out, so an answer that a newer
  // one overtook leaves the newer one in place. Without it, the empty answer
  // to a lookup started before signing in could land after the one started
  // by login(), and sign the reviewer straight back out.
  private lookups = 0;

  /**
   * Whether this build has anywhere to sign in. The demo bundle and a
   * deployment without dcim-authn-api carry no URL, and are left as they were
   * before there was a login: unguarded, with the API's own error showing.
   */
  hasSessionSurface(): boolean {
    return !!this.configService.getConfig().dcimAuthnApiUrl;
  }

  /**
   * Resolves the session, asking dcim-authn-api once for as long as the answer
   * is a reviewer. A missing session is not remembered, so a login that
   * happened since (here or in DCIM) is found by the next caller.
   */
  ensureUser(): Promise<StaffUser | null> {
    const user = this.user();
    if (user) return Promise.resolve(user);
    this.pending ??= this.lookup()
      .then((result) => result.user)
      .finally(() => {
        this.pending = undefined;
      });
    return this.pending;
  }

  /**
   * Signs in with an email address and password, the way the DCIM login does.
   * Throws an error whose message is fit to show the reviewer.
   */
  async login(email: string, password: string): Promise<StaffUser> {
    let response: Response;
    try {
      response = await fetch(this.endpoint('login/password'), {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });
    } catch {
      // A network failure, or a CORS answer that leaves out this origin.
      throw new Error('The sign-in service could not be reached. Try again in a moment.');
    }

    if (!response.ok) {
      throw new Error(StaffSessionService.loginFailureMessage(response.status));
    }

    const { user, failed } = await this.lookup();
    if (failed) {
      // Signed in, but dcim-authn-api (or its directory) did not answer the
      // question who that is. A retry may well work.
      throw new Error(
        'You were signed in, but your account could not be looked up. Try again in a moment.',
      );
    }
    if (!user) {
      // dcim-authn-api answered that there is no session, right after setting
      // one: the cookie is not one this page can send back. A cookie domain
      // that does not cover this host, most likely.
      throw new Error('You were signed in, but the session did not reach this site.');
    }
    return user;
  }

  /**
   * Ends the staff session. dcim-authn-api clears `dcim_auth` on the parent
   * domain, so DCIM is signed out as well. Throws when it did not, leaving the
   * reviewer in place so the header does not claim a sign-out that did not
   * happen.
   */
  async logout(): Promise<void> {
    const response = await fetch(this.endpoint('logout'), {
      method: 'POST',
      credentials: 'include',
    });
    if (!response.ok) {
      throw new Error(`Logout failed: ${response.status} ${response.statusText}`);
    }
    this.sessionEnded();
  }

  /** Forgets the reviewer after an API has said the session is gone. */
  sessionEnded(): void {
    // Supersede any lookup still out, so its answer cannot sign the reviewer
    // back in.
    this.lookups += 1;
    this.user.set(null);
  }

  /**
   * Asks dcim-authn-api who is signed in. For the guard any failure counts as
   * nobody: a reviewer who cannot be identified cannot use the backoffice
   * either, and the login page is where an outage then shows. `failed` tells
   * login() an outage apart from an answer that there is no session.
   */
  private async lookup(): Promise<{ user: StaffUser | null; failed: boolean }> {
    this.lookups += 1;
    const lookup = this.lookups;
    let user: StaffUser | null = null;
    let failed = false;
    try {
      const response = await this.client.getUserInfo({});
      user = response.user ? { id: response.user.id, name: response.user.name } : null;
    } catch (err) {
      if (ConnectError.from(err).code !== Code.Unauthenticated) {
        failed = true;
        // eslint-disable-next-line no-console
        console.warn('Could not read the staff session', err);
      }
    }

    // A newer lookup or a sign-out has taken over: leave its answer standing.
    if (lookup !== this.lookups) return { user: this.user(), failed };
    this.user.set(user);
    return { user, failed };
  }

  // Joined under the configured URL rather than resolved against it, which
  // would drop a path prefix dcim-authn-api may be served under.
  private endpoint(path: string): URL {
    const configured = this.configService.getConfig().dcimAuthnApiUrl ?? '';
    const base = new URL(
      configured.endsWith('/') ? configured : `${configured}/`,
      window.location.origin,
    );
    return new URL(path, base);
  }

  private static loginFailureMessage(status: number): string {
    switch (status) {
      case 400:
      case 401:
        return 'The email address or password is incorrect.';
      case 404:
        // dcim-authn-api only serves the password login where it is enabled.
        return 'Signing in with a password is not enabled in this environment.';
      default:
        return 'The sign-in service did not answer. Try again in a moment.';
    }
  }
}
