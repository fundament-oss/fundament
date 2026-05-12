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
import { ActivatedRoute, Router } from '@angular/router';
import type {
  HostMessage,
  K8sGetRequest,
  K8sListRequest,
  PluginMessage,
} from './postmessage-types';
import type { AllowedResource, KubeResource } from '../types';
import { ConfigService } from '../../config.service';

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
    case 'plugin:k8s:list':
    case 'plugin:k8s:get':
      return true;
    default:
      return false;
  }
}

function isVerbAllowed(
  allowed: AllowedResource[],
  group: string,
  version: string,
  resource: string,
  verb: 'list' | 'get',
): boolean {
  return allowed.some(
    (a) =>
      a.group === group &&
      a.version === version &&
      a.resource === resource &&
      (a.verbs ?? []).includes(verb),
  );
}

function buildResourceUrl(
  base: string,
  clusterId: string,
  args: { group: string; version: string; resource: string; namespace?: string; name?: string },
): string {
  const groupPart = args.group === '' ? `api/${args.version}` : `apis/${args.group}/${args.version}`;
  const nsPart = args.namespace ? `/namespaces/${encodeURIComponent(args.namespace)}` : '';
  const namePart = args.name ? `/${encodeURIComponent(args.name)}` : '';
  return `${base}/clusters/${encodeURIComponent(clusterId)}/${groupPart}${nsPart}/${encodeURIComponent(args.resource)}${namePart}`;
}

function replyForbidden(iframe: HTMLIFrameElement, requestId: string): void {
  iframe.contentWindow?.postMessage(
    {
      type: 'fundament:k8s:result',
      requestId,
      ok: false,
      error: 'forbidden',
    } satisfies HostMessage,
    '*',
  );
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
      sandbox="allow-scripts" intentionally omits allow-same-origin: the iframe runs with an
      opaque origin and cannot send cookies. All cluster data flows through the host-mediated
      broker below (plugin:k8s:list / plugin:k8s:get → fundament:k8s:result), which validates
      every request against the plugin's declared allowedResources.
    -->
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

  allowedResources = input.required<AllowedResource[]>();

  clusterId = input.required<string>();

  resourceName = input<string | undefined>(undefined);

  resourceNamespace = input<string | undefined>(undefined);

  private sanitizer = inject(DomSanitizer);

  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private destroyRef = inject(DestroyRef);

  private configService = inject(ConfigService);

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
        return;
      case 'plugin:resize':
        if (typeof msg.height === 'number' && msg.height > 0) {
          this.frameHeight.set(msg.height);
        }
        return;
      case 'plugin:navigate': {
        // The resource-kind list route is the navigation anchor: from `list`
        // it's the current route, from `detail` it's the parent.
        const baseRoute = this.view() === 'list' ? this.route : this.route.parent;
        this.router.navigate([msg.name], {
          relativeTo: baseRoute,
          queryParams: msg.namespace ? { ns: msg.namespace } : undefined,
        });
        return;
      }
      case 'plugin:k8s:list':
      case 'plugin:k8s:get':
        this.handleK8sRequest(msg, iframe);
        return;
      default: {
        const exhaustive: never = msg;
        throw new Error(`Unhandled plugin message type: ${(exhaustive as PluginMessage).type}`);
      }
    }
  }

  private sendInit(iframe: HTMLIFrameElement): void {
    if (!iframe.contentWindow) return;

    const theme = getCurrentTheme();
    this.lastSentTheme = theme;

    const name = this.resourceName();
    const namespace = this.resourceNamespace();
    const msg: HostMessage = {
      type: 'fundament:init',
      theme,
      pluginName: this.pluginName(),
      crdKind: this.crdKind(),
      view: this.view(),
      ...(name ? { resource: { name, namespace } } : {}),
    };
    iframe.contentWindow.postMessage(msg, '*');
  }

  private async handleK8sRequest(
    msg: K8sListRequest | K8sGetRequest,
    iframe: HTMLIFrameElement,
  ): Promise<void> {
    const isGet = msg.type === 'plugin:k8s:get';
    const verb = isGet ? 'get' : 'list';

    if (!isVerbAllowed(this.allowedResources(), msg.group, msg.version, msg.resource, verb)) {
      // eslint-disable-next-line no-console
      console.warn(`[PluginIframe] rejected ${verb} request not in allowlist`, msg);
      replyForbidden(iframe, msg.requestId);
      return;
    }

    const url = buildResourceUrl(this.kubeApiProxyBase(), this.clusterId(), {
      group: msg.group,
      version: msg.version,
      resource: msg.resource,
      namespace: msg.namespace,
      name: isGet ? msg.name : undefined,
    });

    try {
      const response = await fetch(url, { credentials: 'include' });
      if (!response.ok) {
        iframe.contentWindow?.postMessage(
          {
            type: 'fundament:k8s:result',
            requestId: msg.requestId,
            ok: false,
            error: response.statusText || `HTTP ${response.status}`,
            status: response.status,
          } satisfies HostMessage,
          '*',
        );
        return;
      }
      const data = await response.json();
      const payload: HostMessage = isGet
        ? {
            type: 'fundament:k8s:result',
            requestId: msg.requestId,
            ok: true,
            item: data as KubeResource,
          }
        : {
            type: 'fundament:k8s:result',
            requestId: msg.requestId,
            ok: true,
            items: (data as { items?: KubeResource[] }).items ?? [],
          };
      iframe.contentWindow?.postMessage(payload, '*');
    } catch (err) {
      iframe.contentWindow?.postMessage(
        {
          type: 'fundament:k8s:result',
          requestId: msg.requestId,
          ok: false,
          error: err instanceof Error ? err.message : 'transport error',
        } satisfies HostMessage,
        '*',
      );
    }
  }

  private kubeApiProxyBase(): string {
    return this.configService.getConfig().kubeApiProxyUrl.replace(/\/$/, '');
  }
}
