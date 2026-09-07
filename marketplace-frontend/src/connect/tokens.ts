import { InjectionToken, REQUEST, inject } from '@angular/core';
import { Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { DescService } from '@bufbuild/protobuf';
import { createObservableClient, ObservableClient } from './observable-client';
import createTransferCacheInterceptor from './transfer-cache';
import { ConfigService } from '../app/config.service';
import { CatalogService } from '../generated/catalog/v1/catalog_pb';
import { PublicationService } from '../generated/registry/v1/publication_pb';
import { ReviewService } from '../generated/admin/v1/review_pb';

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

// The two authenticated APIs are called with the visitor's session. In the
// browser that means asking fetch to send cookies cross-origin; during a server
// render there is no cookie jar and `credentials: 'include'` does nothing, so
// the visitor's Cookie header is forwarded from the incoming request instead.
// Must be called in an injection context.
function credentialedFetch(): typeof fetch {
  const request = inject(REQUEST, { optional: true });

  if (!request) {
    return (input, init) => fetch(input, { ...init, credentials: 'include' });
  }

  const cookie = request.headers.get('cookie');

  return (input, init) => {
    const headers = new Headers(init?.headers);
    if (cookie) {
      headers.set('cookie', cookie);
    }
    return fetch(input, { ...init, headers });
  };
}

export const REGISTRY_TRANSPORT = new InjectionToken<Transport>('registry-transport', {
  providedIn: 'root',
  factory: () =>
    createConnectTransport({
      baseUrl: inject(ConfigService).getConfig().registryApiUrl,
      fetch: credentialedFetch(),
    }),
});

export const ADMIN_TRANSPORT = new InjectionToken<Transport>('admin-transport', {
  providedIn: 'root',
  factory: () =>
    createConnectTransport({
      baseUrl: inject(ConfigService).getConfig().adminApiUrl,
      fetch: credentialedFetch(),
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
