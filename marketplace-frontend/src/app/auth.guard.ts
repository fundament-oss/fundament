import { PLATFORM_ID, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { CanActivateFn } from '@angular/router';
import SessionService from './session.service';

/**
 * Keeps the developer portal's routes behind the console's login (FUN-20).
 *
 * Without this, an anonymous visitor reached the portal and every
 * organization-scoped RPC came back `[unauthenticated] no authorization header
 * or auth cookie found`, because nothing had ever started a session. The portal
 * does not own one: it sends the visitor to authn-api, which sets the console's
 * cookie on the parent domain and redirects back here.
 *
 * `window.location` rather than the router: the login lives on another origin,
 * so it is a page load and not a navigation this app can make.
 */
const authGuard: CanActivateFn = async (_route, state) => {
  const session = inject(SessionService);

  // Nothing to sign in against: the demo bundle answers the portal from
  // in-memory fixtures, and a deployment may not have an authn URL wired up
  // either. Let the navigation through and leave the API's own error to
  // surface, as it did before there was a guard.
  if (!session.hasSessionSurface()) {
    return true;
  }

  // Only a browser can be sent to the identity provider. The portal's routes
  // are client-rendered (app.routes.server.registry.ts), so this is belt and
  // braces rather than a case that arises.
  if (!isPlatformBrowser(inject(PLATFORM_ID))) {
    return true;
  }

  if (await session.ensureUser()) {
    return true;
  }

  // The route being activated, not window.location: on an in-app navigation
  // the address bar still holds the page the visitor is leaving.
  session.redirectToLogin(new URL(state.url, window.location.origin).href);
  return false;
};

export default authGuard;
