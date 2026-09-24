import { InjectionToken, inject } from '@angular/core';
import { createConnectTransport } from '@connectrpc/connect-web';
import { createObservableClient, ObservableClient } from './observable-client';
import createTransferCacheInterceptor from './transfer-cache';
import { credentialedFetch } from './credentialed-fetch';
import { ConfigService } from '../app/config.service';
import { OrganizationService } from '../generated/v1/organization_pb';

// organization-api, for the names of the session's organizations: the session
// itself only carries their ids. Apart from tokens.ts for the same reason as
// authn.ts: OrganizationContextService uses it, and tokens.ts depends on that
// service for the registry's Fun-Organization header.
// eslint-disable-next-line import-x/prefer-default-export
export const ORGANIZATION_CLIENT = new InjectionToken<ObservableClient<typeof OrganizationService>>(
  'marketplace-client-organization',
  {
    providedIn: 'root',
    factory: () =>
      createObservableClient(
        OrganizationService,
        createConnectTransport({
          baseUrl: inject(ConfigService).getConfig().organizationApiUrl ?? '',
          fetch: credentialedFetch(),
        }),
        createTransferCacheInterceptor(),
      ),
  },
);
