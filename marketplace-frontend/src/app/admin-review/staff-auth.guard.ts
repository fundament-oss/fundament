import { PLATFORM_ID, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { CanActivateChildFn, Router } from '@angular/router';
import StaffSessionService from './staff-session.service';

/**
 * Keeps the review backoffice behind the staff login. Without it, a reviewer
 * who had not signed in to DCIM first landed on the queue and saw only the
 * API's `unauthenticated` error, with nowhere to sign in.
 *
 * A router redirect rather than a page load, unlike the developer portal's
 * guard: the login is a page of this app, posting to dcim-authn-api itself.
 */
const staffAuthGuard: CanActivateChildFn = async (_route, state) => {
  const session = inject(StaffSessionService);
  const router = inject(Router);

  if (!session.hasSessionSurface()) return true;

  // The backoffice renders on the client only (app.routes.server.admin.ts), so
  // this is belt and braces: a server has no session to look up.
  if (!isPlatformBrowser(inject(PLATFORM_ID))) return true;

  if (await session.ensureUser()) return true;

  return router.createUrlTree(['/login'], { queryParams: { returnUrl: state.url } });
};

export default staffAuthGuard;

/**
 * The page to go to after signing in, from the login's `returnUrl`. Only a
 * path in this app: anything else (another origin, a protocol-relative URL,
 * the login itself) falls back to the queue.
 */
export function safeReturnUrl(returnUrl: string | null | undefined): string {
  if (!returnUrl || !returnUrl.startsWith('/') || /^\/[/\\]/.test(returnUrl)) return '/admin';
  if (returnUrl === '/login' || returnUrl.startsWith('/login?')) return '/admin';
  return returnUrl;
}
