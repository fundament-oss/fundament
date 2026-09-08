// Demo-only application config: the real app, wired to fixtures instead of a
// backend. Never imported by the production entrypoint (main.ts).
import { ApplicationConfig, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideRouter, TitleStrategy } from '@angular/router';
import routes from '../app.routes';
import AppTitleStrategy from '../shell/page-title';
import { ConfigService } from '../config.service';
import AuthService from '../auth.service';
import { DCIM_TRANSPORT } from '../../connect/tokens';
import createDemoTransport from './mock-transport';
import DemoConfigService from './demo-config.service';
import DemoAuthService from './demo-auth.service';
import RackHistoryService from '../racks/rack-history.service';
import DemoRackHistoryService from './demo-rack-history.service';

const demoAppConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes),
    // The demo bootstraps its own config, so anything the real app provides
    // has to stand here too, or the demo quietly runs without it.
    { provide: TitleStrategy, useClass: AppTitleStrategy },
    { provide: ConfigService, useClass: DemoConfigService },
    { provide: AuthService, useClass: DemoAuthService },
    { provide: RackHistoryService, useClass: DemoRackHistoryService },
    { provide: DCIM_TRANSPORT, useFactory: createDemoTransport },
  ],
};

export default demoAppConfig;
