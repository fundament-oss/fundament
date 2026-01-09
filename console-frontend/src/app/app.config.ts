import { ApplicationConfig, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideRouter } from '@angular/router';
import { createConnectTransport } from '@connectrpc/connect-web';
import { AUTHN_TRANSPORT, ORGANIZATION_TRANSPORT } from '../connect/connect.module';
import { PROTO_API_VERSION } from '../proto-version';
import { BehaviorSubject } from 'rxjs';

import { routes } from './app.routes';

const EXPECTED_API_VERSION = PROTO_API_VERSION;

// Global version mismatch observable
export const versionMismatch$ = new BehaviorSubject<boolean>(false);

// Create a version mismatch handler
const handleVersionMismatch = (serverVersion: string) => {
  if (serverVersion && serverVersion !== EXPECTED_API_VERSION) {
    console.warn(`API version mismatch: expected ${EXPECTED_API_VERSION}, got ${serverVersion}`);
    versionMismatch$.next(true);
  }
};

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
        fetch: async (input, init) => {
          const response = await fetch(input, {
            ...init,
            credentials: 'include',
          });

          // Check API version from response header
          const serverVersion = response.headers.get('X-API-Version');
          if (serverVersion) {
            handleVersionMismatch(serverVersion);
          }

          return response;
        },
      }),
    },
  ],
};
