import {
  ApplicationConfig,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import { provideRouter, TitleStrategy } from '@angular/router';

import routes from './app.routes';
import { ConfigService } from './config.service';
import provideStaleChunkReload from './reload-on-stale-chunk';
import AuthService from './auth.service';
import AppTitleStrategy from './shell/page-title';

const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes),
    provideStaleChunkReload(),
    // The tab says where you are, specific first: see page-title.ts.
    { provide: TitleStrategy, useClass: AppTitleStrategy },
    provideAppInitializer(async () => {
      const config = inject(ConfigService);
      const auth = inject(AuthService);
      await config.loadConfig();
      await auth.initializeAuth();
    }),
  ],
};
export default appConfig;
