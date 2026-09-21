import { Injectable, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type { User } from '../generated/authn/v1/authn_pb';
import { AUTHN_CLIENT } from '../connect/authn';
import { ConfigService } from './config.service';

// Set on the way out to the login, cleared as soon as a session resolves, so
// it only ever survives a round trip that came back without one. sessionStorage
// rather than localStorage: it is about this tab's current attempt, and it must
// not outlive the tab.
const LOGIN_ATTEMPT_KEY = 'marketplace_login_attempted';

// sessionStorage is absent during a server render and throws outright in a
// sandboxed frame, and the caller's answer to both is the same, so the access
// is wrapped rather than guarded at each call site.
function readLoginAttempt(): boolean {
  try {
    return sessionStorage.getItem(LOGIN_ATTEMPT_KEY) !== null;
  } catch {
    return false;
  }
}

function writeLoginAttempt(attempted: boolean): void {
  try {
    if (attempted) {
      sessionStorage.setItem(LOGIN_ATTEMPT_KEY, '1');
    } else {
      sessionStorage.removeItem(LOGIN_ATTEMPT_KEY);
    }
  } catch {
    // No storage to remember the attempt in. The guard then falls back to
    // redirecting every time, which is what it did before there was a flag.
  }
}

/**
 * The visitor's console session, as the developer portal sees it (FUN-20).
 *
 * The marketplace mints nothing of its own: the session is the console's
 * `fundament_auth` cookie, set by authn-api on the parent domain, which the
 * browser therefore sends to the portal's own APIs as well —
 * marketplace-registry-api validates exactly that cookie, issuer and audience.
 * So "signing in" here means sending the visitor to authn-api's OIDC login with
 * a `return_to`, and reading the session back out of `GetUserInfo`.
 *
 * There is deliberately no refresh attempt when `GetUserInfo` says no. The
 * cookie *is* the access token, and `/refresh` runs the same validator over the
 * same cookie — requiring a live `exp` — so it can only succeed where
 * `GetUserInfo` already did. The console's auth guard does try it; here it would
 * be a second failing request in front of every anonymous page load.
 */
@Injectable({ providedIn: 'root' })
export default class SessionService {
  private readonly authnClient = inject(AUTHN_CLIENT);

  private readonly configService = inject(ConfigService);

  /** The signed-in user, or null once a lookup has come back empty. */
  readonly user = signal<User | null>(null);

  private loaded?: Promise<User | null>;

  // Read once, on construction: the question is whether the page load this
  // service belongs to arrived back from a login, and nothing outside this
  // service writes the flag while it is alive.
  private triedLogin = readLoginAttempt();

  /**
   * Whether this build has a session surface at all. The storefront and the
   * demo bundle carry no authn URL — the demo answers the registry from
   * in-memory fixtures and has no backend to authenticate against — so there
   * is nobody to ask and nowhere to send a visitor.
   */
  hasSessionSurface(): boolean {
    return !!this.configService.getConfig().authnApiUrl;
  }

  /** Resolves the session, asking authn-api once per app load. */
  ensureUser(): Promise<User | null> {
    this.loaded ??= this.load();
    return this.loaded;
  }

  /**
   * Where to send the browser to sign in. authn-api runs the OIDC round trip,
   * sets the cookie on the parent domain and redirects back to `returnTo`,
   * which it only honours for an origin the deployment serves.
   */
  loginUrl(returnTo: string): string {
    const base = (this.configService.getConfig().authnApiUrl ?? '').replace(/\/+$/, '');
    return `${base}/login?return_to=${encodeURIComponent(returnTo)}`;
  }

  /**
   * Whether this tab has already been sent to the login and come back without
   * a session.
   *
   * `GetUserInfo` failing does not distinguish "no cookie" from "authn did not
   * answer", and a redirect is only the right response to the first. A login
   * that sets a cookie this origin still cannot read — authn 5xx, a
   * `COOKIE_DOMAIN` that misses this host, a CORS header that does not come
   * back — would otherwise loop the tab silently: dex re-approves the session
   * it just minted on every pass, so nothing prompts, and each hop is a fresh
   * page load calling `location.assign`, so the browser's redirect limit never
   * trips either.
   */
  hasTriedLogin(): boolean {
    return this.triedLogin;
  }

  /**
   * Leaves for the login. A page load and not a router navigation: authn-api
   * is another origin. It lives here rather than in the guard so that what the
   * guard decides can be tested without a real browser leaving the page.
   */
  redirectToLogin(returnTo: string): void {
    this.triedLogin = true;
    writeLoginAttempt(true);
    window.location.assign(this.loginUrl(returnTo));
  }

  private async load(): Promise<User | null> {
    if (!this.hasSessionSurface()) {
      return null;
    }

    try {
      const response = await firstValueFrom(this.authnClient.getUserInfo({}));
      const user = response.user ?? null;
      this.user.set(user);
      if (user) {
        // The round trip worked, so a later one may be attempted again.
        this.triedLogin = false;
        writeLoginAttempt(false);
      }
      return user;
    } catch {
      // Not signed in, or authn is unreachable. Either way there is no session
      // to act on; the guard turns this into a login and everything else
      // treats it as an anonymous visitor.
      this.user.set(null);
      return null;
    }
  }
}
