import { ApplicationConfig, inject } from '@angular/core';
import { appConfig } from '../app.config';
import { ConfigService, AppConfiguration } from '../config.service';
import AuthnApiService from '../authn-api.service';
import { TitleService } from '../title.service';
import {
  AUTHN_TRANSPORT,
  MARKETPLACE_TRANSPORT,
  ORGANIZATION_TRANSPORT,
} from '../../connect/connect.module';
import { PRESENTATION_ENABLED } from '../presentation/presentation.tokens';
import createDemoTransport from './mock-transport';
import FakeAuthnApiService from './fake-authn-api.service';
import DemoConfigService from './demo-config.service';
import DemoTitleService from './demo-title.service';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import FakePluginInstallationService from './fake-plugin-installation.service';
import PluginRegistryService from '../plugin-resources/plugin-registry.service';
import FakePluginRegistryService from './fake-plugin-registry.service';
import PluginResourceStoreService from '../plugin-resources/plugin-resource-store.service';
import FakePluginResourceStoreService from './fake-plugin-resource-store.service';

// Dummy URLs — the demo transports are in-memory and ignore baseUrl.
//
// `marketplaceUrl` is the exception: it is a link, not a transport, and the
// marketplace's demo bundle really is served from this origin under
// /marketplace/ (see scripts/build-marketplace-demo.ts), so the plugin sheet's
// "View full details" reaches the storefront instead of falling back to the
// console's own plugin page. The trailing '#' is what makes that bundle's hash
// routes resolve: `marketplacePluginUrl` appends /plugins/<id> to this, and the
// demo build uses `withHashLocation()` because it is static files with no
// rewrite rules behind them. `window` is safe here: the demo entrypoint is
// browser-only.
const demoConfig: AppConfiguration = {
  authnApiUrl: 'demo://authn',
  organizationApiUrl: 'demo://organization',
  marketplaceApiUrl: 'demo://marketplace',
  kubeApiProxyUrl: 'demo://kube',
  pluginProxyUrl: 'demo://plugin-proxy',
  marketplaceUrl: `${window.location.origin}/marketplace/#`,
};

// Reuse the real app providers, then override the backend seams. Later providers win
// in Angular DI, so every RPC client resolves the in-memory transport and the auth
// guard sees a seeded user.
//
// Each fake `implements Pick<Real, ...>` of the service it stands in for. That pins the
// members it lists: change a signature or rename one on the real service and the build
// breaks here rather than the demo at runtime. It does not pin the *set* of members, so
// a method added to the real service still compiles — keep the Pick lists exhaustive by
// hand.
//
// The `as unknown as` casts below are still needed: a class with private members is only
// assignable from itself or a subclass, and the fakes are neither.
const demoAppConfig: ApplicationConfig = {
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
    // While presenting, the slide title owns the document title. A real subclass, so
    // the console's own title format is inherited rather than restated.
    { provide: TitleService, useClass: DemoTitleService },
    // Installs are in-memory, so the walkthrough can install a plugin for real.
    {
      provide: PluginInstallationService,
      useFactory: () => new FakePluginInstallationService() as unknown as PluginInstallationService,
    },
    // Everything the console would otherwise read from the cluster over
    // kube-api-proxy: plugin definitions, CRDs and their objects. Without these
    // the demo fires fetches at demo://kube, which the page's CSP blocks.
    {
      provide: PluginRegistryService,
      useFactory: () => inject(FakePluginRegistryService) as unknown as PluginRegistryService,
    },
    {
      provide: PluginResourceStoreService,
      useFactory: () =>
        inject(FakePluginResourceStoreService) as unknown as PluginResourceStoreService,
    },
    { provide: AUTHN_TRANSPORT, useFactory: () => createDemoTransport() },
    { provide: ORGANIZATION_TRANSPORT, useFactory: () => createDemoTransport() },
    { provide: MARKETPLACE_TRANSPORT, useFactory: () => createDemoTransport() },
    { provide: PRESENTATION_ENABLED, useValue: true },
  ],
};

export default demoAppConfig;
