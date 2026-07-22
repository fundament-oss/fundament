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
  K8sCreateRequest,
  K8sGetRequest,
  K8sListRequest,
  PluginMessage,
} from './postmessage-types';
import type { AllowedResource, KubeResource } from '../types';
import buildResourceUrl from '../kube-url.utils';
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
    case 'plugin:k8s:create':
    case 'plugin:create':
    case 'plugin:navigate-back':
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
  verb: 'list' | 'get' | 'create',
): boolean {
  return allowed.some(
    (a) =>
      a.group === group &&
      a.version === version &&
      a.resource === resource &&
      (a.verbs ?? []).includes(verb),
  );
}

function replyK8sResult(
  iframe: HTMLIFrameElement,
  requestId: string,
  result:
    | { ok: true; items?: KubeResource[]; item?: KubeResource }
    | { ok: false; error: string; status?: number },
): void {
  iframe.contentWindow?.postMessage(
    { type: 'fundament:k8s:result', requestId, ...result } satisfies HostMessage,
    '*',
  );
}

function replyForbidden(iframe: HTMLIFrameElement, requestId: string): void {
  replyK8sResult(iframe, requestId, { ok: false, error: 'forbidden' });
}

function replyTransportError(iframe: HTMLIFrameElement, requestId: string, err: unknown): void {
  replyK8sResult(iframe, requestId, {
    ok: false,
    error: err instanceof Error ? err.message : 'transport error',
  });
}

// Kubernetes reports write rejections (409 conflict, 422 validation/CEL) in the
// Status body's `message`, not in statusText — surface it so the plugin form can
// show the real reason.
async function readK8sError(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { message?: unknown };
    if (typeof body.message === 'string' && body.message) return body.message;
  } catch {
    // body was not JSON; fall through to the generic message
  }
  return response.statusText || `HTTP ${response.status}`;
}

@Component({
  selector: 'app-plugin-iframe',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (status() === 'error') {
      <div
        class="bg-danger-50 text-danger-700 flex items-center gap-2 rounded-md border border-rose-200 px-4 py-3 dark:border-rose-800"
      >
        <span
          >The plugin UI did not load. Check that the plugin is running and includes the SDK.</span
        >
      </div>
    }
    <!--
      The sandbox intentionally omits allow-same-origin: the iframe runs with an opaque origin
      and cannot send cookies. All cluster data flows through the host-mediated broker below
      (plugin:k8s:list / plugin:k8s:get / plugin:k8s:create → fundament:k8s:result), which
      validates every request against the plugin's declared allowedResources.

      allow-forms is required for create UIs: without it the browser blocks form submission and
      the plugin's submit handler never fires. Submits stay in-frame (handlers preventDefault and
      route writes through the broker), so this does not grant the iframe any navigation power.
    -->
    <iframe
      #pluginFrame
      [src]="trustedSrc()"
      sandbox="allow-scripts allow-forms"
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

  view = input.required<'list' | 'detail' | 'create'>();

  allowedResources = input.required<AllowedResource[]>();

  clusterId = input.required<string>();

  resourceName = input<string | undefined>(undefined);

  resourceNamespace = input<string | undefined>(undefined);

  namespaces = input<string[] | undefined>(undefined);

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
      case 'plugin:k8s:list':
      case 'plugin:k8s:get':
        this.handleK8sRequest(msg, iframe);
        return;
      case 'plugin:k8s:create':
        this.handleK8sCreate(msg, iframe);
        return;
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
    const namespaces = this.namespaces();
    const msg: HostMessage = {
      type: 'fundament:init',
      theme,
      pluginName: this.pluginName(),
      crdKind: this.crdKind(),
      view: this.view(),
      ...(name ? { resource: { name, namespace } } : {}),
      ...(namespaces ? { namespaces } : {}),
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
        replyK8sResult(iframe, msg.requestId, {
          ok: false,
          error: response.statusText || `HTTP ${response.status}`,
          status: response.status,
        });
        return;
      }
      const data = await response.json();
      replyK8sResult(
        iframe,
        msg.requestId,
        isGet
          ? { ok: true, item: data as KubeResource }
          : { ok: true, items: (data as { items?: KubeResource[] }).items ?? [] },
      );
    } catch (err) {
      replyTransportError(iframe, msg.requestId, err);
    }
  }

  private async handleK8sCreate(msg: K8sCreateRequest, iframe: HTMLIFrameElement): Promise<void> {
    if (!isVerbAllowed(this.allowedResources(), msg.group, msg.version, msg.resource, 'create')) {
      // eslint-disable-next-line no-console
      console.warn('[PluginIframe] rejected create request not in allowlist', msg);
      replyForbidden(iframe, msg.requestId);
      return;
    }

    const url = buildResourceUrl(this.kubeApiProxyBase(), this.clusterId(), {
      group: msg.group,
      version: msg.version,
      resource: msg.resource,
      namespace: msg.namespace,
    });

    try {
      const response = await fetch(url, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(msg.body),
      });
      if (!response.ok) {
        replyK8sResult(iframe, msg.requestId, {
          ok: false,
          error: await readK8sError(response),
          status: response.status,
        });
        return;
      }
      const data = await response.json();
      replyK8sResult(iframe, msg.requestId, { ok: true, item: data as KubeResource });
    } catch (err) {
      replyTransportError(iframe, msg.requestId, err);
    }
  }

  private kubeApiProxyBase(): string {
    return this.configService.getConfig().kubeApiProxyUrl.replace(/\/$/, '');
  }
}
