import {
  Component,
  ChangeDetectionStrategy,
  input,
  signal,
  computed,
  inject,
  viewChild,
  ElementRef,
  DestroyRef,
  type OnInit,
} from '@angular/core';
import { DomSanitizer, type SafeResourceUrl } from '@angular/platform-browser';
import { Router } from '@angular/router';
import type { HostMessage, PluginMessage } from './postmessage-types';
import { POSTMESSAGE_VERSION } from './postmessage-types';

function getCurrentTheme(): 'light' | 'dark' {
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
}

function isPluginMessage(data: unknown): data is PluginMessage {
  if (typeof data !== 'object' || data === null) return false;
  const msg = data as Record<string, unknown>;
  return (
    typeof msg['type'] === 'string' &&
    (msg['type'] === 'plugin:ready' ||
      msg['type'] === 'plugin:resize' ||
      msg['type'] === 'plugin:navigate')
  );
}

@Component({
  selector: 'app-plugin-iframe',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <iframe
      #pluginFrame
      [src]="trustedSrc()"
      sandbox="allow-scripts"
      [style.height.px]="frameHeight()"
      class="block w-full border-none"
      title="Plugin custom UI"
    ></iframe>
  `,
})
export default class PluginIframeComponent implements OnInit {
  src = input.required<string>();

  pluginName = input.required<string>();

  crdKind = input.required<string>();

  view = input.required<'list' | 'detail'>();

  private sanitizer = inject(DomSanitizer);

  private router = inject(Router);

  private destroyRef = inject(DestroyRef);

  private iframeRef = viewChild<ElementRef<HTMLIFrameElement>>('pluginFrame');

  frameHeight = signal(150);

  trustedSrc = computed<SafeResourceUrl>(() =>
    this.sanitizer.bypassSecurityTrustResourceUrl(this.src()),
  );

  private iframeReady = false;

  ngOnInit(): void {
    const onMessage = (event: MessageEvent): void => {
      const iframe = this.iframeRef()?.nativeElement;
      if (!iframe || event.source !== iframe.contentWindow) return;
      if (!isPluginMessage(event.data)) return;

      this.handleMessage(event.data, iframe);
    };

    const onResize = (): void => {
      if (!this.iframeReady) return;
      const iframe = this.iframeRef()?.nativeElement;
      if (!iframe?.contentWindow) return;

      const msg: HostMessage = { type: 'fundament:resize-requested' };
      iframe.contentWindow.postMessage(msg, '*');
    };

    window.addEventListener('message', onMessage);
    window.addEventListener('resize', onResize);

    const observer = new MutationObserver(() => {
      if (!this.iframeReady) return;
      const iframe = this.iframeRef()?.nativeElement;
      if (!iframe?.contentWindow) return;

      const msg: HostMessage = {
        type: 'fundament:theme-changed',
        theme: getCurrentTheme(),
      };
      iframe.contentWindow.postMessage(msg, '*');
    });

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    });

    this.destroyRef.onDestroy(() => {
      window.removeEventListener('message', onMessage);
      window.removeEventListener('resize', onResize);
      observer.disconnect();
    });
  }

  private handleMessage(msg: PluginMessage, iframe: HTMLIFrameElement): void {
    if (msg.type !== 'plugin:resize') {
      // eslint-disable-next-line no-console
      console.log('[plugin-iframe]', msg);
    }

    switch (msg.type) {
      case 'plugin:ready':
        this.iframeReady = true;
        this.sendInit(iframe);
        break;
      case 'plugin:resize':
        if (typeof msg.height === 'number' && msg.height > 0) {
          this.frameHeight.set(msg.height);
        }
        break;
      case 'plugin:navigate':
        if (typeof msg.path === 'string') {
          this.router.navigateByUrl(msg.path);
        }
        break;
    }
  }

  private sendInit(iframe: HTMLIFrameElement): void {
    if (!iframe.contentWindow) return;

    const msg: HostMessage = {
      type: 'fundament:init',
      version: POSTMESSAGE_VERSION,
      theme: getCurrentTheme(),
      pluginName: this.pluginName(),
      crdKind: this.crdKind(),
      view: this.view(),
    };
    iframe.contentWindow.postMessage(msg, '*');
  }
}
