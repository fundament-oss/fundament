import {
  ApplicationConfig,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import { provideRouter, withRouterConfig, withInMemoryScrolling } from '@angular/router';
import { provideClientHydration, withNoIncrementalHydration } from '@angular/platform-browser';
import routes from './app.routes';
import { ConfigService } from './config.service';

// eslint-disable-next-line import-x/prefer-default-export
export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    // Reuse the server-rendered DOM instead of throwing it away.
    //
    // Deliberately without event replay, which incremental hydration would turn
    // on for us: it works by way of inline scripts, and the app is served with
    // `script-src 'self'`. Weakening that policy is a poor trade for replaying
    // the clicks that land in the moment before hydration, when the design
    // system's web components have not upgraded and would not have handled them
    // anyway. There are no @defer blocks for incremental hydration to hydrate.
    provideClientHydration(withNoIncrementalHydration()),
    provideRouter(
      routes,
      withRouterConfig({ paramsInheritanceStrategy: 'always' }),
      // Scroll to top on navigation and restore position on back/forward.
      withInMemoryScrolling({ scrollPositionRestoration: 'enabled', anchorScrolling: 'enabled' }),
    ),
    // The API base URLs come from /assets/config/config.json, so the transports
    // cannot be constructed until it has been read.
    provideAppInitializer(async () => {
      await inject(ConfigService).loadConfig();
    }),
  ],
};
