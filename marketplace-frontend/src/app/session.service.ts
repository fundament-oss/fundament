import { Injectable, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type { User } from '../generated/authn/v1/authn_pb';
import { AUTHN_CLIENT } from '../connect/authn';
import { ConfigService } from './config.service';

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
   * Leaves for the login. A page load and not a router navigation: authn-api
   * is another origin. It lives here rather than in the guard so that what the
   * guard decides can be tested without a real browser leaving the page.
   */
  redirectToLogin(returnTo: string): void {
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
