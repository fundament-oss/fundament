import { TestBed } from '@angular/core/testing';
import { inject } from '@angular/core';
import { vi } from 'vitest';
import PluginInstallationService from '../plugin-installation/plugin-installation.service';
import PluginRegistryService from '../plugin-resources/plugin-registry.service';
import PluginNavService from '../plugin-resources/plugin-nav.service';
import { PLUGIN_INSTALLS_ENSURE_EVENT } from '../presentation/presentation.tokens';
import FakePluginInstallationService from './fake-plugin-installation.service';
import FakePluginRegistryService from './fake-plugin-registry.service';

// Guards the walkthrough slide that shows the freshly installed plugin in the
// project sidebar: the demo's registry must resolve the in-memory installs into
// menu entries, including the few seconds a new install spends Pending.
describe('demo plugin sidebar', () => {
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

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows no plugin menu for the seeded installs', async () => {
    await TestBed.inject(PluginRegistryService).loadPlugins('cl-production');

    // openfsc and grafana are installed but carry no definition, so they have no
    // console UI — as in production, where the menu comes from the definition.
    expect(TestBed.inject(PluginNavService).projectNav()).toEqual([]);
  });

  it('picks up an install that is still coming up', async () => {
    vi.useFakeTimers();
    const installs = TestBed.inject(PluginInstallationService);
    const registry = TestBed.inject(PluginRegistryService);
    const nav = TestBed.inject(PluginNavService);

    await installs.installPlugin('cl-production', 'cert-manager', 'v1.17.2', 'sha256:demo');
    await registry.loadPlugins('cl-production');

    // Still Pending right after the install: not in the menu yet.
    expect(nav.projectNav()).toEqual([]);

    // The registry keeps reading until the install reports Running.
    await vi.advanceTimersByTimeAsync(5000);

    expect(nav.projectNav()).toEqual([
      {
        pluginName: 'cert-manager',
        label: 'Cert Manager',
        items: [
          { label: 'Certificates', crdPlural: 'certificates.cert-manager.io', icon: 'certificate' },
        ],
      },
    ]);
  });

  it('follows an install that lands while the menu is on screen', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    const nav = TestBed.inject(PluginNavService);

    // The project (and with it the sidebar) is opened before anything is installed.
    await registry.loadPlugins('cl-production');
    expect(nav.projectNav()).toEqual([]);

    // What the slides after the install slide dispatch to stand on their own.
    document.dispatchEvent(new CustomEvent(PLUGIN_INSTALLS_ENSURE_EVENT));
    await Promise.resolve();

    expect(nav.projectNav().map((group) => group.pluginName)).toEqual(['cert-manager']);
  });

  it('resolves the CRD behind a menu entry by reference, plural or kind', () => {
    const registry = TestBed.inject(PluginRegistryService);

    ['certificates.cert-manager.io', 'certificates', 'Certificate'].forEach((key) => {
      expect(registry.getCrd('cert-manager', key, 'cl-production')?.kind).toBe('Certificate');
    });
    expect(registry.getCrd('cert-manager', 'issuers', 'cl-production')).toBeUndefined();
  });
});
