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
    @if (status() === 'error') {
      <div
        class="flex items-center gap-2 rounded-md border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-800 dark:bg-rose-950 dark:text-rose-300"
      >
        <span
          >The plugin UI did not load. Check that the plugin is running and includes the SDK.</span
        >
      </div>
    }
    <iframe
      #pluginFrame
      [src]="trustedSrc()"
      sandbox="allow-scripts"
      [style.height.px]="frameHeight()"
      [class]="status() === 'error' ? 'hidden' : 'block w-full border-none'"
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

  status = signal<'loading' | 'ready' | 'error'>('loading');

  // Required: Angular blocks all iframe [src] bindings by default. The bypass is safe here
  // because src() is always a backend-controlled URL (e.g. /plugin-ui/...), never user input.
  trustedSrc = computed<SafeResourceUrl>(() =>
    this.sanitizer.bypassSecurityTrustResourceUrl(this.src()),
  );

  private lastSentTheme: 'light' | 'dark' | null = null;

  ngOnInit(): void {
    const readyTimeout = setTimeout(() => {
      if (this.status() === 'loading') this.status.set('error');
    }, 5000);

    this.destroyRef.onDestroy(() => clearTimeout(readyTimeout));

    const onMessage = (event: MessageEvent): void => {
      const iframe = this.iframeRef()?.nativeElement;
      if (!iframe || event.source !== iframe.contentWindow) return;
      if (!isPluginMessage(event.data)) return;

      this.handleMessage(event.data, iframe);
    };

    window.addEventListener('message', onMessage);

    const observer = new MutationObserver(() => {
      if (this.status() !== 'ready') return;
      const iframe = this.iframeRef()?.nativeElement;
      if (!iframe?.contentWindow) return;

      const theme = getCurrentTheme();
      if (theme === this.lastSentTheme) return;
      this.lastSentTheme = theme;

      const msg: HostMessage = {
        type: 'fundament:theme-changed',
        theme,
      };
      iframe.contentWindow.postMessage(msg, '*');
    });

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    });

    this.destroyRef.onDestroy(() => {
      window.removeEventListener('message', onMessage);
      observer.disconnect();
    });
  }

  private handleMessage(msg: PluginMessage, iframe: HTMLIFrameElement): void {
    switch (msg.type) {
      case 'plugin:ready':
        this.status.set('ready');
        this.sendInit(iframe);
        break;
      case 'plugin:resize':
        if (typeof msg.height === 'number' && msg.height > 0) {
          this.frameHeight.set(msg.height);
        }
        break;
      case 'plugin:navigate':
        if (typeof msg.path === 'string' && msg.path.startsWith('/plugins/')) {
          this.router.navigateByUrl(msg.path);
        }
        break;
      default:
        throw new Error(`Unhandled plugin message type: ${(msg as PluginMessage).type}`);
    }
  }

  private sendInit(iframe: HTMLIFrameElement): void {
    if (!iframe.contentWindow) return;

    const theme = getCurrentTheme();
    this.lastSentTheme = theme;

    const msg: HostMessage = {
      type: 'fundament:init',
      theme,
      pluginName: this.pluginName(),
      crdKind: this.crdKind(),
      view: this.view(),
    };
    iframe.contentWindow.postMessage(msg, '*');
  }
}
