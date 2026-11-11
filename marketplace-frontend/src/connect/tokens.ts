import { InjectionToken, inject } from '@angular/core';
import { Code, ConnectError, Interceptor, Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { DescService } from '@bufbuild/protobuf';
import { Router } from '@angular/router';
import { createObservableClient, ObservableClient } from './observable-client';
import createTransferCacheInterceptor from './transfer-cache';
import { ConfigService } from '../app/config.service';
import { CatalogService } from '../generated/catalog/v1/catalog_pb';
import { PublicationService } from '../generated/registry/v1/publication_pb';
import { ReviewService } from '../generated/admin/v1/review_pb';
import OrganizationContextService from '../app/organization-context.service';
import StaffSessionService from '../app/admin-review/staff-session.service';
import { credentialedFetch } from './credentialed-fetch';

// One transport per deployable (FUN-20): the storefront, the developer's
// publishing API and the backoffice are three services behind three hosts.

// The catalog is anonymous and internet-facing, so it deliberately sends no
// credentials: `credentials: 'include'` would force the server into
// non-wildcard CORS, which is the coupling a credential-free catalog avoids.
export const CATALOG_TRANSPORT = new InjectionToken<Transport>('catalog-transport', {
  providedIn: 'root',
  factory: () =>
    createConnectTransport({ baseUrl: inject(ConfigService).getConfig().catalogApiUrl }),
});

// Every registry.v1 RPC is organization-scoped through the Fun-Organization
// header (FUN-6). The active organization is resolved lazily from the
// session's membership list, so the first RPC waits for it.
function registryFetch(): typeof fetch {
  const base = credentialedFetch();
  const organizationContext = inject(OrganizationContextService);

  return async (input, init) => {
    const organizationId = await organizationContext.ensureOrganizationId();
    if (!organizationId) return base(input, init);
    const headers = new Headers(init?.headers);
    headers.set('Fun-Organization', organizationId);
    return base(input, { ...init, headers });
  };
}

export const REGISTRY_TRANSPORT = new InjectionToken<Transport>('registry-transport', {
  providedIn: 'root',
  factory: () =>
    createConnectTransport({
      baseUrl: inject(ConfigService).getConfig().registryApiUrl,
      fetch: registryFetch(),
    }),
});

// A staff session that runs out while the backoffice is open (the token
// expires, or the reviewer signs out in DCIM) sends the reviewer back to the
// login, and then on to the page they were on, rather than leaving the API's
// `unauthenticated` on screen with nowhere to sign in. The error still reaches
// the caller.
function staffSessionInterceptor(): Interceptor {
  const router = inject(Router);
  const session = inject(StaffSessionService);

  return (next) => async (req) => {
    try {
      return await next(req);
    } catch (err) {
      if (
        session.hasSessionSurface() &&
        ConnectError.from(err).code === Code.Unauthenticated &&
        !router.url.startsWith('/login')
      ) {
        session.sessionEnded();
        router.navigate(['/login'], { queryParams: { returnUrl: router.url } }).catch(() => {});
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

function createClientToken<T extends DescService>(
  service: T,
  transportToken: InjectionToken<Transport>,
): InjectionToken<ObservableClient<T>> {
  return new InjectionToken<ObservableClient<T>>(`marketplace-client-${service.typeName}`, {
    providedIn: 'root',
    factory: () =>
      createObservableClient(service, inject(transportToken), createTransferCacheInterceptor()),
  });
}

export const CATALOG_CLIENT = createClientToken(CatalogService, CATALOG_TRANSPORT);
export const PUBLICATION_CLIENT = createClientToken(PublicationService, REGISTRY_TRANSPORT);
export const REVIEW_CLIENT = createClientToken(ReviewService, ADMIN_TRANSPORT);
