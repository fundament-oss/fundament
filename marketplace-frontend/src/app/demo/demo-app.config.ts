import {
  ApplicationConfig,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import {
  provideRouter,
  withRouterConfig,
  withInMemoryScrolling,
  withHashLocation,
} from '@angular/router';
import routes from '../app.routes';
import { ConfigService, CONFIG_LOADER, AppConfiguration } from '../config.service';
import { CATALOG_TRANSPORT, REGISTRY_TRANSPORT } from '../../connect/tokens';
import { createDemoCatalogTransport, createDemoRegistryTransport } from './mock-transport';

// Dummy URLs — the demo transports are in-memory and ignore baseUrl.
// `adminApiUrl` is required by the type but unreachable: `demoRoutes` drops the
// routes that would build the admin transport.
//
// `consoleUrl` is the console demo's own origin, because that is where this
// bundle is served from: `build-marketplace-demo.ts` drops it under
// /marketplace/ on the console, and both demos seed the same plugin ids, so a
// listing's "Install plugin" button lands on a console page that really
// resolves.
//
// It stays set inside the deck's iframe too, which is what the slide narrates:
// installing happens in the console. The stage then shows the console's plugin
// page, loaded in this frame as its own instance, and the next slide is a
// console slide that hides the frame, so the deck is back to one console by
// itself.
//
// `present=0` is what keeps that instance a plain console: the demo build
// presents by default, so without it the console arrives running its own
// walkthrough over the page the visitor asked for — in a frame that narrow,
// nothing but the deck's "works best on a large screen" notice. The storefront
// carries the query through to the plugin page. `window` is safe at module
// scope: this entrypoint is browser-only and never server-rendered.
const demoConfig: AppConfiguration = {
  catalogApiUrl: 'demo://catalog',
  registryApiUrl: 'demo://registry',
  adminApiUrl: 'demo://admin',
  consoleUrl: `${window.location.origin}/?present=0`,
};

// The backoffice is not part of the walkthrough and has no in-memory transport,
// so its routes are dropped rather than left to build the real one: opening
// /admin in the demo would call demo://admin for real, which `connect-src
// 'self'` blocks — a console error and a dead screen. The wildcard route sends
// those paths back to the storefront. Stubbing ReviewService here instead would
// make the routes work again, and is the change to make if the deck ever shows
// a reviewer's queue.
const demoRoutes = routes.filter((route) => !route.path?.startsWith('admin'));

// Written out rather than spread over `appConfig`: the demo needs a different
// router (hash location, see below) and no hydration, which leaves almost
// nothing of the real config to reuse. The two that matter — the routes and the
// config seam — are still shared, so a new route reaches the demo for free
// unless it needs a backend the demo has no transport for (see `demoRoutes`).
//
// Hash location is what keeps the bundle deployment-agnostic. It is served as
// static files under /marketplace/ inside the console demo, so every deep link
// has to resolve to the one index.html that is actually on disk; a path route
// like /marketplace/plugins/x would need a rewrite rule in both nginx and
// Vercel.
const demoAppConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(
      demoRoutes,
      withHashLocation(),
      withRouterConfig({ paramsInheritanceStrategy: 'always' }),
      withInMemoryScrolling({ scrollPositionRestoration: 'enabled', anchorScrolling: 'enabled' }),
    ),
    { provide: CONFIG_LOADER, useValue: async () => demoConfig },
    provideAppInitializer(async () => {
      await inject(ConfigService).loadConfig();
    }),
    { provide: CATALOG_TRANSPORT, useFactory: createDemoCatalogTransport },
    { provide: REGISTRY_TRANSPORT, useFactory: createDemoRegistryTransport },
  ],
};

export default demoAppConfig;
