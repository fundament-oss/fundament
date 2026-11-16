import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router } from '@angular/router';
import { vi } from 'vitest';
import PluginIframeComponent from './plugin-iframe.component';
import PluginRegistryService from '../plugin-registry.service';
import { ConfigService } from '../../config.service';
import { PluginAuthService } from '../../plugin-auth/plugin-auth.service';

const PROXY = 'https://plugins.test';
const PAGE = `${PROXY}/clusters/c1/plugins/acme--ceph/v1/console/diskpools-list.html`;

const plugin = {
  installationName: 'acme--ceph',
  installationVersion: 'v1',
  installationId: 'uid-1',
  label: 'Ceph Storage',
  customComponents: {
    DiskPool: { list: 'diskpools-list.html', create: 'diskpools-create.html' },
  },
};

async function openPage(extraInputs: Record<string, unknown> = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.reject(new Error('no network in tests'))),
  );
  const navigate = vi.fn();
  await TestBed.configureTestingModule({
    imports: [PluginIframeComponent],
    providers: [
      { provide: Router, useValue: { navigate } },
      { provide: ActivatedRoute, useValue: { parent: null } },
      { provide: PluginRegistryService, useValue: { getPlugin: () => plugin } },
      {
        provide: ConfigService,
        useValue: { getConfig: () => ({ pluginProxyUrl: PROXY, kubeApiProxyUrl: '' }) },
      },
      {
        provide: PluginAuthService,
        useValue: {
          retain: () => {},
          release: () => {},
          acquire: async () => ({ token: 't', expiresAt: Date.now() + 600_000 }),
          requestRefresh: async () => {},
          tokenSignal: () => signal(null),
          failedSignal: () => signal(false),
        },
      },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(PluginIframeComponent);
  const inputs: Record<string, string> = {
    src: PAGE,
    pluginName: 'acme--ceph',
    pluginVersion: 'v1',
    installationId: 'uid-1',
    crdKind: 'DiskPool',
    view: 'list',
    clusterId: 'c1',
  };
  Object.entries({ ...inputs, ...extraInputs }).forEach(([name, value]) =>
    fixture.componentRef.setInput(name, value),
  );
  fixture.detectChanges();
  return { fixture, navigate };
}

/** The page's own frame, as a plugin message's source must be. */
function pageFrame(fixture: ComponentFixture<PluginIframeComponent>): HTMLIFrameElement {
  return (fixture.nativeElement as HTMLElement).querySelector('iframe')!;
}

function fromPlugin(fixture: ComponentFixture<PluginIframeComponent>, data: unknown): void {
  window.dispatchEvent(
    new MessageEvent('message', {
      source: pageFrame(fixture).contentWindow,
      origin: PROXY,
      data,
    }),
  );
  fixture.detectChanges();
}

/** The frames inside the sheet, which the sheet directive moves to the body. */
function sheetFrames(): HTMLIFrameElement[] {
  return Array.from(document.querySelectorAll<HTMLIFrameElement>('nldd-sheet iframe'));
}

describe('PluginIframeComponent sheets', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    document.querySelectorAll('nldd-sheet').forEach((sheet) => sheet.remove());
  });

  it("opens the plugin's own create page in a console sheet", async () => {
    const { fixture } = await openPage();

    fromPlugin(fixture, { type: 'plugin:open-sheet', view: 'create' });

    const frames = sheetFrames();
    expect(frames).toHaveLength(1);
    expect(frames[0].getAttribute('src')).toBe(
      `${PROXY}/clusters/c1/plugins/acme--ceph/v1/console/diskpools-create.html`,
    );
    // The same sandbox as the page under it, so nothing about a sheet widens it.
    expect(frames[0].getAttribute('sandbox')).toBe(pageFrame(fixture).getAttribute('sandbox'));
  });

  it('refuses a page the plugin does not declare', async () => {
    const { fixture } = await openPage();

    fromPlugin(fixture, { type: 'plugin:open-sheet', view: 'edit', resource: { name: 'pool' } });
    fromPlugin(fixture, { type: 'plugin:open-sheet', view: 'create', kind: 'Secret' });

    expect(sheetFrames()).toHaveLength(0);
  });

  it('ignores a request that does not come from its own frame', async () => {
    const { fixture } = await openPage();

    window.dispatchEvent(
      new MessageEvent('message', {
        source: window,
        origin: PROXY,
        data: { type: 'plugin:open-sheet', view: 'create' },
      }),
    );
    fixture.detectChanges();

    expect(sheetFrames()).toHaveLength(0);
  });

  it('opens one sheet at a time', async () => {
    const { fixture } = await openPage();

    fromPlugin(fixture, { type: 'plugin:open-sheet', view: 'create' });
    fromPlugin(fixture, { type: 'plugin:open-sheet', view: 'create' });

    expect(sheetFrames()).toHaveLength(1);
  });

  it('titles the sheet itself, and says which plugin it is from', async () => {
    const { fixture } = await openPage();

    fromPlugin(fixture, { type: 'plugin:open-sheet', view: 'create', title: 'Sign in again' });

    const bar = document.querySelector('nldd-sheet nldd-top-title-bar')!;
    expect(bar.getAttribute('text')).toBe('New disk pool');
    expect(bar.getAttribute('supporting-text')).toBe('From the Ceph Storage plugin');
  });
});

describe('PluginIframeComponent init message', () => {
  afterEach(() => vi.unstubAllGlobals());

  async function sendInit(extraInputs: Record<string, unknown>) {
    const { fixture } = await openPage(extraInputs);
    const postMessage = vi.fn();
    const iframe = { contentWindow: { postMessage } } as unknown as HTMLIFrameElement;
    const instance = fixture.componentInstance as unknown as {
      sendInit: (frame: HTMLIFrameElement) => Promise<void>;
    };
    await instance.sendInit(iframe);
    return postMessage.mock.calls[0][0];
  }

  it('sends projectId and namespaces when given', async () => {
    const msg = await sendInit({ projectId: 'p1', namespaces: ['a', 'b'] });
    expect(msg.type).toBe('fundament:init');
    expect(msg.projectId).toBe('p1');
    expect(msg.namespaces).toEqual(['a', 'b']);
  });

  it('omits projectId when not given', async () => {
    const msg = await sendInit({});
    expect('projectId' in msg).toBe(false);
  });
});
