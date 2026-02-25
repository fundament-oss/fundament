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
import { provideNgIconsConfig, provideIcons } from '@ng-icons/core';
import pluginIcons from './plugin-resources/generated-plugin-icons';
import { AUTHN_TRANSPORT, ORGANIZATION_TRANSPORT } from '../connect/connect.module';
import EXPECTED_API_VERSION from '../proto-version';
import routes from './app.routes';
import { ConfigService } from './config.service';
import OrganizationContextService from './organization-context.service';
import PluginRegistryService from './plugin-resources/plugin-registry.service';

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

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes, withRouterConfig({ paramsInheritanceStrategy: 'always' })),
    // Initialize configuration before app starts
    provideAppInitializer(() => {
      const configService = inject(ConfigService);
      return configService.loadConfig();
    }),
    // Load plugin definitions from YAML files
    provideAppInitializer(() => {
      const pluginRegistry = inject(PluginRegistryService);
      return pluginRegistry.loadPlugins();
    }),
    provideNgIconsConfig({
      size: '1rem', // Default icon size
    }),
    provideIcons(pluginIcons),
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
    // Provide the Organization transport
    {
      provide: ORGANIZATION_TRANSPORT,
      useFactory: (injector: Injector) => {
        const configService = inject(ConfigService);
        const config = configService.getConfig();
        return createConnectTransport({
          baseUrl: config.organizationApiUrl,
          fetch: async (input, init) => {
            // Get the current organization ID from the context service
            const orgId = runInInjectionContext(injector, () => {
              const contextService = inject(OrganizationContextService);
              return contextService.currentOrganizationId();
            });

            // Add the Fun-Organization header if we have an organization selected
            const headers = new Headers(init?.headers);
            if (orgId) {
              headers.set('Fun-Organization', orgId);
            }

            const response = await fetch(input, {
              ...init,
              headers,
              credentials: 'include',
            });

            // Check API version from response header
            const serverVersion = response.headers.get('X-API-Version');
            if (serverVersion) {
              handleVersionMismatch(serverVersion);
            }

            return response;
          },
        });
      },
      deps: [Injector],
    },
  ],
};
