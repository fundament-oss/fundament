import {
  ApplicationConfig,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import { provideRouter, withRouterConfig, withInMemoryScrolling } from '@angular/router';
import routes from './app.routes';
import { ConfigService } from './config.service';

// eslint-disable-next-line import-x/prefer-default-export
export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
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
