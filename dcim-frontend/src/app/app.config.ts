import { ApplicationConfig, inject, provideAppInitializer, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideRouter } from '@angular/router';

import routes from './app.routes';
import { ConfigService } from './config.service';
import AuthService from './auth.service';

const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes),
    provideAppInitializer(async () => {
      await inject(ConfigService).loadConfig();
      await inject(AuthService).initializeAuth();
    }),
  ],
};
export default appConfig;
