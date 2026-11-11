import { InjectionToken, inject } from '@angular/core';
import { Client, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { credentialedFetch } from './credentialed-fetch';
import { ConfigService } from '../app/config.service';
import { AuthnService } from '../generated/dcimauthn/v1/dcimauthn_pb';

// The staff session behind the review backoffice (see StaffSessionService).
// A promise client rather than an observable one: the backoffice renders on
// the client only (app.routes.server.admin.ts), so there is no server answer
// for the transfer cache to hand over.
// eslint-disable-next-line import-x/prefer-default-export
export const DCIM_AUTHN_CLIENT = new InjectionToken<Client<typeof AuthnService>>(
  'marketplace-client-dcim-authn',
  {
    providedIn: 'root',
    factory: () =>
      createClient(
        AuthnService,
        createConnectTransport({
          baseUrl: inject(ConfigService).getConfig().dcimAuthnApiUrl ?? '',
          fetch: credentialedFetch(),
        }),
      ),
  },
);
