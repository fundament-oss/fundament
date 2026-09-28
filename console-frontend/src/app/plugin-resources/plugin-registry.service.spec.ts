import { TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import PluginRegistryService from './plugin-registry.service';
import { ConfigService } from '../config.service';
import { PLUGIN } from '../../connect/tokens';
import type { PluginInstallationItem } from './types';

function installation(
  phase: string | undefined,
  ready = phase === 'Running',
): PluginInstallationItem {
  return {
    metadata: { name: 'acme--cert-manager', uid: 'uid-1' },
    spec: {
      definitionRef: {
        organizationName: 'acme',
        pluginName: 'cert-manager',
        pluginVersion: 'v1.0.0',
        definitionHash: 'sha256:abc',
      },
    },
    status:
      phase === undefined
        ? (undefined as unknown as PluginInstallationItem['status'])
        : { phase, ready },
  };
}

// Guards the sidebar picking up a plugin that finishes installing after the
// project was opened: the menu is built on opening, which is usually while a
// plugin just installed is still deploying.
describe('PluginRegistryService', () => {
  let items: PluginInstallationItem[];
  let getPluginDefinition: ReturnType<typeof vi.fn>;
  let getConfig: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.useFakeTimers();
    items = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ items }), { status: 200 })),
    );
    getPluginDefinition = vi.fn(() =>
      of({
        definition: {
          metadata: { name: 'cert-manager', displayName: 'Cert Manager', version: 'v1.0.0' },
          menu: { project: [{ crd: 'certificates.cert-manager.io', label: '', icon: '' }] },
          crds: ['certificates.cert-manager.io'],
          customComponents: {},
          allowedResources: [],
        },
      }),
    );

    getConfig = vi.fn(() => ({ kubeApiProxyUrl: 'https://proxy.test' }));

    TestBed.configureTestingModule({
      providers: [
        {
          provide: ConfigService,
          useValue: { getConfig } as unknown as ConfigService,
        },
        { provide: PLUGIN, useValue: { getPluginDefinition } },
      ],
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('adds a plugin to the menu once its installation reports running', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Deploying')];

    await registry.loadPlugins('cl-1');
    expect(registry.allPlugins()).toEqual([]);

    items = [installation('Running')];
    await vi.advanceTimersByTimeAsync(5000);

    expect(registry.allPlugins().map((p) => p.installationName)).toEqual(['acme--cert-manager']);
  });

  it('treats an installation the controller has not picked up yet as on its way', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation(undefined)];

    await registry.loadPlugins('cl-1');
    items = [installation('Running')];
    await vi.advanceTimersByTimeAsync(5000);

    expect(registry.allPlugins()).toHaveLength(1);
  });

  it('stops reading once nothing is on its way, and does not refetch an unchanged menu', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];

    await registry.loadPlugins('cl-1');
    expect(registry.allPlugins()).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(20000);

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(getPluginDefinition).toHaveBeenCalledTimes(1);
  });

  it('backs off a list that keeps failing', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    vi.mocked(fetch).mockImplementation(async () => new Response('', { status: 403 }));

    await registry.loadPlugins('cl-1');
    await vi.advanceTimersByTimeAsync(5000);
    expect(fetch).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(9999);
    expect(fetch).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it('retries a definition that keeps failing no slower than the idle pace', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];
    getPluginDefinition.mockImplementation(() => throwError(() => new Error('gone')));

    await registry.loadPlugins('cl-1');
    // 5s, 10s, 20s, then capped at 30s.
    await vi.advanceTimersByTimeAsync(5000 + 10000 + 20000 + 30000);
    expect(fetch).toHaveBeenCalledTimes(5);
    await vi.advanceTimersByTimeAsync(30000);
    expect(fetch).toHaveBeenCalledTimes(6);
  });

  it('starts over after a first load that threw', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];
    getConfig.mockImplementationOnce(() => {
      throw new Error('no config yet');
    });

    await expect(registry.loadPlugins('cl-1')).rejects.toThrow('no config yet');
    await registry.loadPlugins('cl-1');

    expect(registry.allPlugins()).toHaveLength(1);
  });

  it('stops reading when the project is left', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Deploying')];

    await registry.loadPlugins('cl-1');
    registry.reset();

    items = [installation('Running')];
    await vi.advanceTimersByTimeAsync(20000);

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(registry.allPlugins()).toEqual([]);
  });
});
