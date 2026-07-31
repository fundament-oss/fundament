import {
  ApplicationConfig,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import { provideRouter, withNavigationErrorHandler } from '@angular/router';

import routes from './app.routes';
import { recoverFromChunkLoadError } from './chunk-load-recovery';
import { ConfigService } from './config.service';
import AuthService from './auth.service';

const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(
      routes,
      // A deploy replaces the content-hashed bundles, so a tab still running
      // the old app fails to lazy-load route chunks; recover via a full page
      // load of the destination url.
      withNavigationErrorHandler(recoverFromChunkLoadError),
    ),
    provideAppInitializer(async () => {
      const config = inject(ConfigService);
      const auth = inject(AuthService);
      await config.loadConfig();
      await auth.initializeAuth();
    }),
  ],
};
export default appConfig;
