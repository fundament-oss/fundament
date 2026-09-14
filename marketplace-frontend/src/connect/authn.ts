import { InjectionToken, inject } from '@angular/core';
import { Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { createObservableClient, ObservableClient } from './observable-client';
import createTransferCacheInterceptor from './transfer-cache';
import { credentialedFetch } from './credentialed-fetch';
import { ConfigService } from '../app/config.service';
import { AuthnService } from '../generated/authn/v1/authn_pb';

// The session surface backing the developer portal's organization context.
// Lives apart from tokens.ts so OrganizationContextService can use this
// client while tokens.ts uses the service for the registry transport's
// Fun-Organization header — importing it from there would be a cycle.
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
