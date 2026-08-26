import { ApplicationConfig, REQUEST_CONTEXT, inject, mergeApplicationConfig } from '@angular/core';
import { provideServerRendering, withRoutes } from '@angular/ssr';
import { appConfig } from './app.config';
import serverRoutes from './app.routes.server';
import { CONFIG_LOADER, ConfigLoader, EMPTY_CONFIGURATION } from './config.service';
import ServerContext from './server-context';

const serverOverrides: ApplicationConfig = {
  providers: [
    provideServerRendering(withRoutes(serverRoutes)),
    // The browser fetches /assets/config/config.json; under Node that relative
    // URL has no origin, so the Node server reads the file once at start-up and
    // hands the result to each render through REQUEST_CONTEXT. It also swaps in
    // the in-cluster API base URLs, so a server render talks to the APIs
    // directly instead of hairpinning back out through the gateway.
    {
      provide: CONFIG_LOADER,
      useFactory: (): ConfigLoader => {
        const context = inject(REQUEST_CONTEXT) as ServerContext | null;
        const config = context?.config ?? EMPTY_CONFIGURATION;
        return async () => config;
      },
    },
  ],
};

const serverAppConfig = mergeApplicationConfig(appConfig, serverOverrides);

export default serverAppConfig;
