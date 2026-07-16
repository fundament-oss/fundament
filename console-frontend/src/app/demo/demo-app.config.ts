import { ApplicationConfig } from '@angular/core';
import { appConfig } from '../app.config';
import { ConfigService, AppConfiguration } from '../config.service';
import AuthnApiService from '../authn-api.service';
import { TitleService } from '../title.service';
import { AUTHN_TRANSPORT, ORGANIZATION_TRANSPORT } from '../../connect/connect.module';
import { PRESENTATION_ENABLED } from '../presentation/presentation.tokens';
import { createDemoTransport } from './mock-transport';
import { FakeAuthnApiService } from './fake-authn-api.service';
import { DemoConfigService } from './demo-config.service';
import { DemoTitleService } from './demo-title.service';

// Dummy URLs — the demo transports are in-memory and ignore baseUrl.
const demoConfig: AppConfiguration = {
  authnApiUrl: 'demo://authn',
  organizationApiUrl: 'demo://organization',
  kubeApiProxyUrl: 'demo://kube',
};

// Reuse the real app providers, then override the backend seams. Later providers win
// in Angular DI, so every RPC client resolves the in-memory transport and the auth
// guard sees a seeded user.
export const demoAppConfig: ApplicationConfig = {
  providers: [
    ...appConfig.providers,
    {
      provide: ConfigService,
      useFactory: () => new DemoConfigService(demoConfig) as unknown as ConfigService,
    },
    {
      provide: AuthnApiService,
      useFactory: () => new FakeAuthnApiService() as unknown as AuthnApiService,
    },
    // While presenting, the slide title owns the document title.
    {
      provide: TitleService,
      useFactory: () => new DemoTitleService() as unknown as TitleService,
    },
    { provide: AUTHN_TRANSPORT, useFactory: () => createDemoTransport() },
    { provide: ORGANIZATION_TRANSPORT, useFactory: () => createDemoTransport() },
    { provide: PRESENTATION_ENABLED, useValue: true },
  ],
};
