import { ApplicationConfig, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideRouter } from '@angular/router';
import { createConnectTransport } from '@connectrpc/connect-web';
import { AUTHN_TRANSPORT, ORGANIZATION_TRANSPORT } from '../connect/connect.module';

import { routes } from './app.routes';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes),
    // Provide the Authn transport
    {
      provide: AUTHN_TRANSPORT,
      useValue: createConnectTransport({
        baseUrl: 'http://authn.127.0.0.1.nip.io:8080',
        fetch: (input, init) => {
          return fetch(input, {
            ...init,
            credentials: 'include', // Include the HTTP-only authentication cookie with requests, also below
          });
        },
      }),
    },
    // Provide the Organization transport
    {
      provide: ORGANIZATION_TRANSPORT,
      useValue: createConnectTransport({
        baseUrl: 'http://organization.127.0.0.1.nip.io:8080',
        fetch: (input, init) => {
          return fetch(input, {
            ...init,
            credentials: 'include',
          });
        },
      }),
    },
  ],
};
