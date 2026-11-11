import { Injectable, inject, signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';
import { DCIM_AUTHN_CLIENT } from '../../connect/dcim-authn';
import endpoint from '../../connect/endpoint';
import { ConfigService } from '../config.service';

export interface StaffUser {
  id: string;
  name: string;
}

/** What dcim-authn-api said about the session. `failed` tells an outage (it
 *  could not be asked, or could not answer) apart from an answer that there is
 *  no session: `user` is null in both. */
export interface StaffSession {
  user: StaffUser | null;
  failed: boolean;
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

  // The lookup still out, which every caller joins rather than starting one of
  // its own. login() replaces it with a fresh one, since an older lookup was
  // asked before the cookie existed.
  private pending?: Promise<StaffSession>;

  // Bumped by every lookup and by every sign-out, so an answer that a newer
  // one overtook leaves the newer one in place. Without it, the empty answer
  // to a lookup started before signing in could land after the one started
  // by login(), and sign the reviewer straight back out.
  private lookups = 0;

  // Set when marketplace-admin-api refused a session that dcim-authn-api may
  // still accept (see sessionRefused), and taken by the login page it leads to.
  private refused = false;

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
    return this.ensureSession().then((session) => session.user);
  }

  /** As ensureUser, but telling an outage apart from no session at all. */
  ensureSession(): Promise<StaffSession> {
    const user = this.user();
    if (user) return Promise.resolve({ user, failed: false });
    return this.pending ?? this.startLookup();
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

    // A fresh lookup, which callers from here on join: one already out was
    // asked before the cookie existed.
    this.refused = false;
    const { user, failed } = await this.startLookup();
    if (failed) {
      // Signed in, but dcim-authn-api (or its directory) did not answer the
      // question who that is. A retry may well work.
      throw new Error(
        'You were signed in, but your account could not be looked up. Try again in a moment.',
      );
    }
    if (!user) {
      // dcim-authn-api answered that there is no session, right after setting
      // one: the browser did not send the cookie back to it from this page. A
      // cookie domain that does not cover dcim-authn-api's host, or a browser
      // that blocks the cookie on a cross-site call. Whether the cookie also
      // reaches marketplace-admin-api only shows on the first review call: a
      // refusal there leads back to the login page, which then says so (see
      // sessionRefused).
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

  /** Forgets the reviewer after signing out. */
  sessionEnded(): void {
    // Supersede any lookup still out, so its answer cannot sign the reviewer
    // back in.
    this.lookups += 1;
    this.pending = undefined;
    this.user.set(null);
  }

  /**
   * Forgets the reviewer after marketplace-admin-api refused the session. Most
   * often it ran out, and dcim-authn-api says the same, so the login page
   * shows its form. But dcim-authn-api may still accept it (a cookie domain
   * that leaves out marketplace-admin-api's host, a JWT secret that differs):
   * the login page would then send the reviewer straight back to the page
   * that was just refused, and round again. The flag lets it say what
   * happened instead.
   */
  sessionRefused(): void {
    this.refused = true;
    this.sessionEnded();
  }

  /** Whether marketplace-admin-api refused the session since the last
   *  sign-in. Asking clears it, so only the login page it led to sees it. */
  takeRefusal(): boolean {
    const { refused } = this;
    this.refused = false;
    return refused;
  }

  private startLookup(): Promise<StaffSession> {
    const lookup: Promise<StaffSession> = this.lookup().finally(() => {
      if (this.pending === lookup) this.pending = undefined;
    });
    this.pending = lookup;
    return lookup;
  }

  /** Asks dcim-authn-api who is signed in. */
  private async lookup(): Promise<StaffSession> {
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

  private endpoint(path: string): URL {
    return endpoint(this.configService.getConfig().dcimAuthnApiUrl, path);
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
