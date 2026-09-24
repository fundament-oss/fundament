import { InjectionToken, inject } from '@angular/core';
import { Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { createObservableClient, ObservableClient } from './observable-client';
import createTransferCacheInterceptor from './transfer-cache';
import { credentialedFetch } from './credentialed-fetch';
import { ConfigService } from '../app/config.service';
import { AuthnService } from '../generated/authn/v1/authn_pb';

// The session surface backing the developer portal's login and organization
// context. Lives apart from tokens.ts so SessionService can use this client
// while tokens.ts reaches the session through OrganizationContextService for
// the registry transport's Fun-Organization header — importing it from there
// would be a cycle.
export const AUTHN_TRANSPORT = new InjectionToken<Transport>('authn-transport', {
  providedIn: 'root',
  factory: () =>
    createConnectTransport({
      baseUrl: inject(ConfigService).getConfig().authnApiUrl ?? '',
      fetch: credentialedFetch(),
    }),
});

export const AUTHN_CLIENT = new InjectionToken<ObservableClient<typeof AuthnService>>(
  'marketplace-client-authn',
  {
    providedIn: 'root',
    factory: () =>
      createObservableClient(
        AuthnService,
        inject(AUTHN_TRANSPORT),
        createTransferCacheInterceptor(),
      ),
  },
);
