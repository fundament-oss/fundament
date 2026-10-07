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

/** What the apiserver returns for the menu's CRD, cut down to what is read. */
const CRD = { spec: { names: { kind: 'Certificate' } } };

function isCrdRead(url: unknown): boolean {
  return String(url).includes('customresourcedefinitions');
}

/** Reads of a CRD, which the menu's labels need: one per CRD, not on a clock. */
function crdReads(): number {
  return vi.mocked(fetch).mock.calls.filter((call) => isCrdRead(call[0])).length;
}

/** Reads of the installation list, apart from the CRD reads the menu's labels
 *  need: those are a one-off per CRD and not on the list's clock. */
function listReads(): number {
  return vi.mocked(fetch).mock.calls.filter((call) => !isCrdRead(call[0])).length;
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
      vi.fn(async (url: RequestInfo | URL) =>
        isCrdRead(url)
          ? new Response(JSON.stringify(CRD), { status: 200 })
          : new Response(JSON.stringify({ items }), { status: 200 }),
      ),
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

    expect(listReads()).toBe(1);
    expect(getPluginDefinition).toHaveBeenCalledTimes(1);
  });

  it("names a menu entry's CRD by the kind the CRD declares", async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];

    await registry.loadPlugins('cl-1');
    await vi.advanceTimersByTimeAsync(0);

    // "certificates.cert-manager.io" alone can only give "Certificates"; the
    // kind is what a label like "DNS Endpoints" needs.
    expect(registry.crdKind('certificates.cert-manager.io')).toBe('Certificate');
  });

  it('leaves the kind unknown when the CRD cannot be read', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];
    vi.mocked(fetch).mockImplementation(async (url) =>
      isCrdRead(url)
        ? new Response('', { status: 403 })
        : new Response(JSON.stringify({ items }), { status: 200 }),
    );

    await registry.loadPlugins('cl-1');
    await vi.advanceTimersByTimeAsync(0);

    // The menu still stands: the label falls back to the CRD reference.
    expect(registry.allPlugins()).toHaveLength(1);
    expect(registry.crdKind('certificates.cert-manager.io')).toBeUndefined();
  });

  it('reads a kind again on the next poll when the CRD was not there yet', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];
    // Right after an install the CRD is often not served yet. The menu does not
    // change when it appears, so the read has to be retried on its own.
    vi.mocked(fetch).mockImplementation(async (url) => {
      if (!isCrdRead(url)) return new Response(JSON.stringify({ items }), { status: 200 });
      // mock.calls already holds this read, so the first one sees a count of 1.
      return crdReads() > 1
        ? new Response(JSON.stringify(CRD), { status: 200 })
        : new Response('', { status: 404 });
    });

    await registry.loadPlugins('cl-1');
    await vi.advanceTimersByTimeAsync(0);
    expect(registry.crdKind('certificates.cert-manager.io')).toBeUndefined();

    await vi.advanceTimersByTimeAsync(30000);

    expect(registry.crdKind('certificates.cert-manager.io')).toBe('Certificate');
  });

  it('does not read a kind it already knows again', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];

    await registry.loadPlugins('cl-1');
    await vi.advanceTimersByTimeAsync(30000 * 3);

    expect(registry.crdKind('certificates.cert-manager.io')).toBe('Certificate');
    expect(crdReads()).toBe(1);
  });

  it('backs off a list that keeps failing', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    vi.mocked(fetch).mockImplementation(async () => new Response('', { status: 403 }));

    await registry.loadPlugins('cl-1');
    await vi.advanceTimersByTimeAsync(5000);
    expect(listReads()).toBe(2);
    await vi.advanceTimersByTimeAsync(9999);
    expect(listReads()).toBe(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(listReads()).toBe(3);
  });

  it('retries a definition that keeps failing no slower than the idle pace', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];
    getPluginDefinition.mockImplementation(() => throwError(() => new Error('gone')));

    await registry.loadPlugins('cl-1');
    // 5s, 10s, 20s, then capped at 30s.
    await vi.advanceTimersByTimeAsync(5000 + 10000 + 20000 + 30000);
    expect(listReads()).toBe(5);
    await vi.advanceTimersByTimeAsync(30000);
    expect(listReads()).toBe(6);
  });

  it('backs off a failing definition while another installation is still on its way', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [
      installation('Running'),
      { ...installation('Deploying'), metadata: { name: 'acme--other', uid: 'uid-2' } },
    ];
    getPluginDefinition.mockImplementation(() => throwError(() => new Error('gone')));

    await registry.loadPlugins('cl-1');
    // The list is read every 5s; the definition only at 5s, then 15s.
    await vi.advanceTimersByTimeAsync(5000 + 10000);
    expect(listReads()).toBe(4);
    expect(getPluginDefinition).toHaveBeenCalledTimes(3);
  });

  it('retries a failed read at once when the plugins are asked for again', async () => {
    const registry = TestBed.inject(PluginRegistryService);
    items = [installation('Running')];
    vi.mocked(fetch).mockImplementationOnce(async () => new Response('', { status: 403 }));

    await registry.loadPlugins('cl-1');
    expect(registry.allPlugins()).toEqual([]);

    await registry.loadPlugins('cl-1');
    expect(listReads()).toBe(2);
    expect(registry.allPlugins()).toHaveLength(1);

    // A read that went through is not repeated on the next ask.
    await registry.loadPlugins('cl-1');
    expect(listReads()).toBe(2);
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

    expect(listReads()).toBe(1);
    expect(registry.allPlugins()).toEqual([]);
  });
});
