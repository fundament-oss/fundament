import {
  ApplicationConfig,
  provideBrowserGlobalErrorListeners,
  provideAppInitializer,
  inject,
  Injector,
  runInInjectionContext,
} from '@angular/core';
import { provideRouter, withRouterConfig } from '@angular/router';
import { createConnectTransport } from '@connectrpc/connect-web';
import { BehaviorSubject } from 'rxjs';
import {
  AUTHN_TRANSPORT,
  INSTALL_TRANSPORT,
  MARKETPLACE_TRANSPORT,
  ORGANIZATION_TRANSPORT,
} from '../connect/connect.module';
import EXPECTED_API_VERSION from '../proto-version.gen';
import routes from './app.routes';
import { ConfigService } from './config.service';
import OrganizationContextService from './organization-context.service';

// Global version mismatch observable
export const versionMismatch$ = new BehaviorSubject<boolean>(false);

// Create a version mismatch handler
const handleVersionMismatch = (serverVersion: string) => {
  if (serverVersion && serverVersion !== EXPECTED_API_VERSION) {
    // eslint-disable-next-line no-console
    console.warn(`API version mismatch: expected ${EXPECTED_API_VERSION}, got ${serverVersion}`);
    versionMismatch$.next(true);
  }
};

// The fetch every organization-scoped surface shares: the HTTP-only
// authentication cookie, plus the Fun-Organization header once an
// organization is selected. onResponse lets a surface inspect the response.
const organizationFetch =
  (injector: Injector, onResponse?: (response: Response) => void): typeof fetch =>
  async (input, init) => {
    const orgId = runInInjectionContext(injector, () =>
      inject(OrganizationContextService).currentOrganizationId(),
    );
    const headers = new Headers(init?.headers);
    if (orgId) {
      headers.set('Fun-Organization', orgId);
    }
    const response = await fetch(input, { ...init, headers, credentials: 'include' });
    onResponse?.(response);
    return response;
  };

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes, withRouterConfig({ paramsInheritanceStrategy: 'always' })),
    // Initialize configuration before app starts
    provideAppInitializer(() => {
      const configService = inject(ConfigService);
      return configService.loadConfig();
    }),
    // Provide the Authn transport
    {
      provide: AUTHN_TRANSPORT,
      useFactory: () => {
        const configService = inject(ConfigService);
        const config = configService.getConfig();
        return createConnectTransport({
          baseUrl: config.authnApiUrl,
          fetch: (input, init) =>
            fetch(input, {
              ...init,
              credentials: 'include', // Include the HTTP-only authentication cookie with requests, also below
            }),
        });
      },
    },
    // Provide the Marketplace transport. The catalog is unauthenticated: no
    // cookie, no Fun-Organization header, no version handshake.
    {
      provide: MARKETPLACE_TRANSPORT,
      useFactory: () => {
        const configService = inject(ConfigService);
        return createConnectTransport({ baseUrl: configService.getConfig().marketplaceApiUrl });
      },
    },
    // Provide the Install transport: install.v1 on the marketplace host, but
    // credentialed and organization-scoped like organization-api (FUN-22).
    // No API-version check: EXPECTED_API_VERSION hashes organization-api's
    // protos, not install.v1's.
    {
      provide: INSTALL_TRANSPORT,
      useFactory: (injector: Injector) => {
        const configService = inject(ConfigService);
        return createConnectTransport({
          baseUrl: configService.getConfig().marketplaceApiUrl,
          fetch: organizationFetch(injector),
        });
      },
      deps: [Injector],
    },
    // Provide the Organization transport, which also checks the API version
    // the server reports against the protos this console was built with.
    {
      provide: ORGANIZATION_TRANSPORT,
      useFactory: (injector: Injector) => {
        const configService = inject(ConfigService);
        return createConnectTransport({
          baseUrl: configService.getConfig().organizationApiUrl,
          fetch: organizationFetch(injector, (response) => {
            const serverVersion = response.headers.get('X-API-Version');
            if (serverVersion) {
              handleVersionMismatch(serverVersion);
            }
          }),
        });
      },
      deps: [Injector],
    },
  ],
};
