import { TestBed } from '@angular/core/testing';
import { inject } from '@angular/core';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import PluginRegistryService from '../plugin-resources/plugin-registry.service';
import FakePluginInstallationService from '../demo/fake-plugin-installation.service';
import FakePluginRegistryService from '../demo/fake-plugin-registry.service';
import { TOURS } from './tours';

interface PluginResourceRoute {
  slideId: string;
  installationName: string;
  crdKey: string;
}

/**
 * Every `plugin-resources` route across the tours, taken apart into the segments
 * the console reads them as. `:pluginName` is the installation name, not the
 * catalog name — a route naming "cert-manager" instead of "system--cert-manager"
 * resolves no CRD and renders an empty list mid-presentation.
 */
function pluginResourceRoutes(): PluginResourceRoute[] {
  return Object.values(TOURS).flatMap((tour) =>
    tour.slides.flatMap((slide) => {
      const match = slide.route?.match(/\/plugin-resources\/([^/]+)\/([^/?]+)/);
      if (!match) return [];
      return [{ slideId: `${tour.id}/${slide.id}`, installationName: match[1], crdKey: match[2] }];
    }),
  );
}

// The tours are content, but their routes are addresses into the console, and a
// wrong one only shows up as an error pane halfway through a presentation.
describe('tour plugin routes', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        // The same overrides demo-app.config.ts applies.
        {
          provide: PluginInstallationService,
          useFactory: () =>
            new FakePluginInstallationService() as unknown as PluginInstallationService,
        },
        {
          provide: PluginRegistryService,
          useFactory: () => inject(FakePluginRegistryService) as unknown as PluginRegistryService,
        },
      ],
    });
  });

  it('addresses a plugin whose CRD the demo registry resolves', () => {
    const registry = TestBed.inject(PluginRegistryService);
    const routes = pluginResourceRoutes();

    // A tour that stopped showing plugin screens would pass every assertion below
    // without checking anything.
    expect(routes.length).toBeGreaterThan(0);

    routes.forEach(({ slideId, installationName, crdKey }) => {
      expect(
        registry.getCrd(installationName, crdKey, 'cl-production'),
        `slide ${slideId}: no CRD for ${installationName}/${crdKey}`,
      ).toBeDefined();
    });
  });

  it('never parks a slide on a project route with no page of its own', () => {
    Object.values(TOURS).forEach((tour) => {
      // A full-bleed slide carries no route at all; it hides the console rather
      // than showing it an address.
      tour.slides.forEach((slide) => {
        if (!slide.route) return;
        // `/projects/:id` slots an empty pane reading "No selection"; every slide
        // that shows the console wants a real page beside the menu.
        expect(
          slide.route,
          `slide ${tour.id}/${slide.id} lands on a project with nothing open`,
        ).not.toMatch(/^\/projects\/[^/]+$/);
      });
    });
  });
});
