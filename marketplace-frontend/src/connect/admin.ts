import { InjectionToken, inject } from '@angular/core';
import { Code, ConnectError, Interceptor, Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { Router } from '@angular/router';
import { ConfigService } from '../app/config.service';
import { ReviewService } from '../generated/admin/v1/review_pb';
import StaffSessionService from '../app/admin-review/staff-session.service';
import { credentialedFetch } from './credentialed-fetch';
import { createClientToken } from './tokens';

// The review backoffice's transport (FUN-20). Apart from tokens.ts, which the
// storefront and the portal share, so only the backoffice's own chunk pulls in
// StaffSessionService and the dcim-authn-api client behind it.

/** Whether a URL is the login page, with or without its query. */
function isLoginUrl(url: string): boolean {
  return url === '/login' || url.startsWith('/login?');
}

// A staff session that runs out while the backoffice is open (the token
// expires, or the reviewer signs out in DCIM) sends the reviewer back to the
// login, and then on to the page they were on, rather than leaving the API's
// `unauthenticated` on screen with nowhere to sign in. The error still reaches
// the caller.
//
// The login is told it was a refusal (`refused`), in the URL rather than in
// the session service, so it survives a reload and nothing is left behind
// when this navigation is cancelled. dcim-authn-api may still accept the
// session marketplace-admin-api just refused (a cookie domain that leaves out
// marketplace-admin-api's host, a JWT secret that differs): the login page
// would then send the reviewer straight back to the page that was refused,
// round and round, and says what happened instead.
export function staffSessionInterceptor(): Interceptor {
  const router = inject(Router);
  const session = inject(StaffSessionService);

  return (next) => async (req) => {
    try {
      return await next(req);
    } catch (err) {
      if (session.hasSessionSurface() && ConnectError.from(err).code === Code.Unauthenticated) {
        // Where the reviewer is going rather than router.url, which is still
        // the page before while a navigation runs: a call made on the way to a
        // submission must bring them back to that submission. A navigation to
        // the login already under way (another call failing at the same time)
        // is left to finish rather than cancelled by one more.
        const navigation = router.currentNavigation();
        const target = navigation
          ? router.serializeUrl(navigation.finalUrl ?? navigation.extractedUrl)
          : router.url;
        if (!isLoginUrl(target)) {
          session.sessionEnded();
          router
            .navigate(['/login'], { queryParams: { returnUrl: target, refused: 1 } })
            .catch(() => {});
        }
      }
      throw err;
    }
  };
}

export const ADMIN_TRANSPORT = new InjectionToken<Transport>('admin-transport', {
  providedIn: 'root',
  factory: () =>
    createConnectTransport({
      baseUrl: inject(ConfigService).getConfig().adminApiUrl,
      fetch: credentialedFetch(),
      interceptors: [staffSessionInterceptor()],
    }),
});

export const REVIEW_CLIENT = createClientToken(ReviewService, ADMIN_TRANSPORT);
