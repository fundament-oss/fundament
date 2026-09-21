import { PLATFORM_ID, inject } from '@angular/core';
import { Location, isPlatformBrowser } from '@angular/common';
import { CanActivateFn } from '@angular/router';
import SessionService from './session.service';

/**
 * Keeps the developer portal's routes behind the console's login (FUN-20).
 *
 * Without this, an anonymous visitor reached the portal and every
 * organization-scoped RPC came back `[unauthenticated] no authorization header
 * or auth cookie found`, because nothing had ever started a session. The portal
 * does not own one: it sends the visitor to a login that sets the console's
 * cookie on the parent domain and comes back here (SessionService.loginUrl
 * picks which).
 *
 * `window.location` rather than the router: every login lives on another
 * origin, so it is a page load and not a navigation this app can make.
 */
const authGuard: CanActivateFn = async (_route, state) => {
  const session = inject(SessionService);
  const location = inject(Location);

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

  // Already been round the login once this tab and come back with no session,
  // so sending the visitor again would only do the same thing (see
  // SessionService.hasTriedLogin). Let the navigation through instead and let
  // the API's own error say what is wrong, which is what the portal did before
  // there was a guard at all.
  if (session.hasTriedLogin()) {
    return true;
  }

  // The route being activated, not window.location: on an in-app navigation
  // the address bar still holds the page the visitor is leaving.
  // prepareExternalUrl puts back whatever the deployment's base href strips
  // off, which the router URL does not carry.
  session.redirectToLogin(location.prepareExternalUrl(state.url));
  return false;
};

export default authGuard;
