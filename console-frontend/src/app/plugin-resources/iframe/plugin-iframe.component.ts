import {
  Component,
  ChangeDetectionStrategy,
  Injector,
  input,
  signal,
  computed,
  effect,
  inject,
  viewChild,
  ElementRef,
  DestroyRef,
  type OnInit,
} from '@angular/core';
import { DomSanitizer, type SafeResourceUrl } from '@angular/platform-browser';
import { ActivatedRoute, Router } from '@angular/router';
import type { HostMessage, PluginMessage } from './postmessage-types';
import { ConfigService } from '../../config.service';
import { PluginAuthService } from '../../plugin-auth/plugin-auth.service';

function getCurrentTheme(): 'light' | 'dark' {
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
}

function isPluginMessage(data: unknown): data is PluginMessage {
  if (typeof data !== 'object' || data === null) return false;
  const msg = data as Record<string, unknown>;
  switch (msg['type']) {
    case 'plugin:ready':
    case 'plugin:resize':
    case 'plugin:navigate':
    case 'plugin:create':
    case 'plugin:navigate-back':
    case 'plugin:request-token-refresh':
      return true;
    default:
      return false;
  }
}

@Component({
  selector: 'app-plugin-iframe',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (status() === 'error') {
      <div
        class="bg-danger-50 text-danger-700 dark:bg-danger-950 dark:text-danger-300 flex items-center gap-2 rounded-md border border-rose-200 px-4 py-3 dark:border-rose-800"
      >
        <span
          >The plugin UI did not load. Check that the plugin is running and includes the SDK.</span
        >
      </div>
    }
    <!--
      FUN-17 sandbox: allow-same-origin is REQUIRED because the iframe runs on the
      dedicated plugin-proxy origin (a real, cross-site origin from the console).
      script-src 'self' in the plugin CSP needs a real origin to resolve, and
      targetOrigin pinning on postMessage needs a checkable origin at both ends.
      Cross-site SOP still blocks any parent.document access; the HttpOnly user
      cookie is unreachable.

      allow-forms remains for plugin create UIs whose submits stay in-frame.
      allow-top-navigation and allow-popups stay ungranted.
    -->
    <iframe
      #pluginFrame
      [src]="trustedSrc()"
      sandbox="allow-scripts allow-same-origin allow-forms"
      [style.height.px]="frameHeight()"
      [class]="status() === 'error' ? 'hidden' : 'block w-full border-none'"
      title="Plugin custom UI"
    ></iframe>
  `,
})
export default class PluginIframeComponent implements OnInit {
  src = input.required<string>();

  pluginName = input.required<string>();

  pluginVersion = input.required<string>();

  installationId = input.required<string>();

  crdKind = input.required<string>();

  view = input.required<'list' | 'detail' | 'create'>();

  clusterId = input.required<string>();

  resourceName = input<string | undefined>(undefined);

  resourceNamespace = input<string | undefined>(undefined);

  namespaces = input<string[] | undefined>(undefined);

  private sanitizer = inject(DomSanitizer);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private destroyRef = inject(DestroyRef);

  private injector = inject(Injector);

  private configService = inject(ConfigService);

  private auth = inject(PluginAuthService);

  private iframeRef = viewChild<ElementRef<HTMLIFrameElement>>('pluginFrame');

  private pluginProxyOrigin = computed(() => {
    // Derive the expected postMessage origin from the ACTUAL iframe src. The
    // src is always the plugin-proxy asset URL (buildPluginConsoleUrl pins
    // every path to the plugin-proxy origin), so this resolves to that
    // cross-site origin.
    //
    // Resolving against window.location.href normalizes the absolute src onto
    // its own origin. Normalizing via URL.origin also strips any trailing
    // slash — MessageEvent.origin is bare per the HTML spec, and postMessage
    // rejects any targetOrigin that isn't a valid origin.
    const src = this.src();
    try {
      return new URL(src, window.location.href).origin;
    } catch {
      // Unresolvable src — fall back to same-origin so postMessage remains
      // functional, but log so ops sees the misconfiguration.
      // eslint-disable-next-line no-console
      console.error(`[plugin-iframe] cannot resolve origin for src: ${src}`);
      return window.location.origin;
    }
  });

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

    // Register this iframe as a live consumer of the (cluster, install) token.
    // Paired with the release() in onDestroy below so PluginAuthService only
    // tears the tuple down once the last iframe on this key is gone.
    this.auth.retain(this.clusterId(), this.installationId());

    // Kick off the first mint in parallel with the iframe load. The signal
    // effect below pushes token-refreshed / auth-failed to the iframe once
    // it's ready.
    void this.auth.acquire(this.clusterId(), this.installationId()).catch(() => {
      // Persistent failure sets the signal to null; the effect below sends
      // fundament:auth-failed.
    });

    // Push refreshed tokens (or auth-failed on persistent failure) to the
    // iframe every time the token signal changes AFTER the plugin is ready.
    // effect() must run in an injection context; ngOnInit isn't one, so pass
    // the component's Injector explicitly.
    //
    // The (snap === null && failed === false) case is the initial state
    // before the first mint resolves — we MUST NOT dispatch auth-failed
    // there or the plugin-sdk rejects every getToken() waiter (including
    // the fetches queued while init is in flight) with 'mint_failed'.
    effect(
      () => {
        const snap = this.auth.tokenSignal(this.clusterId(), this.installationId())();
        const failed = this.auth.failedSignal(this.clusterId(), this.installationId())();
        const iframe = this.iframeRef()?.nativeElement;
        if (!iframe?.contentWindow || this.status() !== 'ready') return;

        if (snap) {
          this.postToIframe(iframe, {
            type: 'fundament:token-refreshed',
            token: snap.token,
            tokenExpiresAt: snap.expiresAt,
          });
          return;
        }
        if (failed) {
          this.postToIframe(iframe, { type: 'fundament:auth-failed', reason: 'mint_failed' });
        }
        // Otherwise snap is null but failed is false: initial in-flight
        // state — do nothing, wait for sendInit's own dispatch.
      },
      { injector: this.injector },
    );

    const onMessage = (event: MessageEvent): void => {
      const iframe = this.iframeRef()?.nativeElement;
      if (!iframe || event.source !== iframe.contentWindow) return;
      // FUN-17: pin the inbound origin to the plugin-proxy site the iframe
      // was navigated to. If the iframe was redirected off that origin, drop
      // the message.
      if (event.origin !== this.pluginProxyOrigin()) return;
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

      this.postToIframe(iframe, {
        type: 'fundament:theme-changed',
        theme,
      });
    });

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    });

    this.destroyRef.onDestroy(() => {
      window.removeEventListener('message', onMessage);
      observer.disconnect();
      this.auth.release(this.clusterId(), this.installationId());
    });
  }

  private postToIframe(iframe: HTMLIFrameElement, msg: HostMessage): void {
    // FUN-17: pin the target origin to plugin-proxy. Because the iframe runs
    // with allow-same-origin on the real plugin-proxy site, the browser
    // refuses delivery if the iframe was navigated off that origin.
    iframe.contentWindow?.postMessage(msg, this.pluginProxyOrigin());
  }

  private handleMessage(msg: PluginMessage, iframe: HTMLIFrameElement): void {
    switch (msg.type) {
      case 'plugin:ready':
        this.status.set('ready');
        void this.sendInit(iframe);
        return;
      case 'plugin:resize':
        if (typeof msg.height === 'number' && msg.height > 0) {
          this.frameHeight.set(msg.height);
        }
        return;
      case 'plugin:navigate': {
        const queryParams = msg.namespace ? { ns: msg.namespace } : undefined;
        if (this.view() === 'create') {
          // The create route is `:resourceKind/create`; the created resource's
          // detail is its sibling `:resourceKind/<name>`.
          this.router.navigate(['..', msg.name], { relativeTo: this.route, queryParams });
          return;
        }
        // The resource-kind list route is the navigation anchor: from `list`
        // it's the current route, from `detail` it's the parent.
        const baseRoute = this.view() === 'list' ? this.route : this.route.parent;
        this.router.navigate([msg.name], { relativeTo: baseRoute, queryParams });
        return;
      }
      case 'plugin:create':
        // Only meaningful from a list view: the create route is the sibling
        // `:resourceKind/create` of the current `:resourceKind` list route. From
        // a detail view it would resolve to a non-existent nested route, so ignore.
        if (this.view() === 'list') {
          this.router.navigate(['create'], { relativeTo: this.route });
        }
        return;
      case 'plugin:navigate-back':
        // Only meaningful from a create/detail view: go up to the `:resourceKind`
        // list. From a list view there is no "back", and `..` would overshoot the
        // resource kind, so ignore it.
        if (this.view() !== 'list') {
          this.router.navigate(['..'], { relativeTo: this.route });
        }
        return;
      case 'plugin:request-token-refresh':
        // Plugin saw a 401 upstream. Force a mint out-of-band; the resulting
        // token-refreshed dispatch from the effect above unblocks the plugin's
        // pending getToken() waiter. requestRefresh's rejection is already
        // reflected in failedSignal so we don't need to act on it here.
        this.auth.requestRefresh(this.clusterId(), this.installationId()).catch(() => undefined);
        return;
      default: {
        const exhaustive: never = msg;
        throw new Error(`Unhandled plugin message type: ${(exhaustive as PluginMessage).type}`);
      }
    }
  }

  private async sendInit(iframe: HTMLIFrameElement): Promise<void> {
    if (!iframe.contentWindow) return;

    const snap = await this.auth.acquire(this.clusterId(), this.installationId()).catch(() => null);

    if (!snap) {
      this.postToIframe(iframe, { type: 'fundament:auth-failed', reason: 'mint_failed' });
      return;
    }

    const theme = getCurrentTheme();
    this.lastSentTheme = theme;

    const name = this.resourceName();
    const namespace = this.resourceNamespace();
    const namespaces = this.namespaces();
    this.postToIframe(iframe, {
      type: 'fundament:init',
      protocolVersion: 1,
      theme,
      pluginName: this.pluginName(),
      crdKind: this.crdKind(),
      view: this.view(),
      ...(name ? { resource: { name, namespace } } : {}),
      ...(namespaces ? { namespaces } : {}),
      kubeApiProxyUrl: this.configService.getConfig().kubeApiProxyUrl,
      clusterId: this.clusterId(),
      token: snap.token,
      tokenExpiresAt: snap.expiresAt,
    });
  }
}
