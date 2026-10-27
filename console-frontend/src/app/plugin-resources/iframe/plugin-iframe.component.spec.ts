import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { describe, it, expect, vi } from 'vitest';
import PluginIframeComponent from './plugin-iframe.component';
import { PluginAuthService } from '../../plugin-auth/plugin-auth.service';
import { ConfigService } from '../../config.service';

type SendInit = (iframe: HTMLIFrameElement) => Promise<void>;

function setup(inputs: Record<string, unknown>) {
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      {
        provide: PluginAuthService,
        useValue: { acquire: () => Promise.resolve({ token: 't', expiresAt: 1 }) },
      },
      {
        provide: ConfigService,
        useValue: { getConfig: () => ({ kubeApiProxyUrl: 'https://proxy.example' }) },
      },
    ],
  });
  const fixture = TestBed.createComponent(PluginIframeComponent);
  const required = {
    src: '/plugin-ui/x.html',
    pluginName: 'x',
    pluginVersion: '1',
    installationId: 'i',
    crdKind: 'K',
    view: 'list',
    clusterId: 'c1',
  };
  Object.entries({ ...required, ...inputs }).forEach(([k, v]) =>
    fixture.componentRef.setInput(k, v),
  );
  const postMessage = vi.fn();
  const iframe = { contentWindow: { postMessage } } as unknown as HTMLIFrameElement;
  const instance = fixture.componentInstance as unknown as { sendInit: SendInit };
  return { sendInit: () => instance.sendInit(iframe), postMessage };
}

describe('PluginIframeComponent init message', () => {
  it('sends projectId and namespaces when given', async () => {
    const { sendInit, postMessage } = setup({ projectId: 'p1', namespaces: ['a', 'b'] });
    await sendInit();
    const msg = postMessage.mock.calls[0][0];
    expect(msg.type).toBe('fundament:init');
    expect(msg.projectId).toBe('p1');
    expect(msg.namespaces).toEqual(['a', 'b']);
  });

  it('omits projectId when not given', async () => {
    const { sendInit, postMessage } = setup({});
    await sendInit();
    expect('projectId' in postMessage.mock.calls[0][0]).toBe(false);
  });
});
