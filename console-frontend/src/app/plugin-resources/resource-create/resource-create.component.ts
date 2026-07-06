import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  effect,
  untracked,
  OnInit,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import PluginIframeComponent from '../iframe/plugin-iframe.component';
import PluginRegistryService from '../plugin-registry.service';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import { TitleService } from '../../title.service';
import { ConfigService } from '../../config.service';
import { NAMESPACE } from '../../../connect/tokens';
import { ListProjectNamespacesRequestSchema } from '../../../generated/v1/namespace_pb';
import type { ParsedCrd } from '../types';
import buildPluginConsoleUrl from '../plugin-console-url.utils';

@Component({
  selector: 'app-resource-create',
  imports: [RouterLink, PluginIframeComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './resource-create.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceCreateComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private titleService = inject(TitleService);

  private registry = inject(PluginRegistryService);

  protected clusterContext = inject(KubeClusterContextService);

  private loader = inject(KubePluginLoaderService);

  private config = inject(ConfigService);

  private namespaceClient = inject(NAMESPACE);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  protected pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  // Present only on the project-level route (projects/:id/...); empty at org level.
  private projectId = computed(() => this.routeParams().get('id') ?? '');

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  customUIUrl = computed(() => {
    const kind = this.crdDef()?.kind;
    if (!kind) return null;
    const plugin = this.plugin();
    const path = plugin?.customComponents?.[kind]?.create;
    const clusterId = this.clusterContext.selectedClusterId();
    if (!plugin || !path || !clusterId) return null;
    return buildPluginConsoleUrl({
      pluginProxyUrl: this.config.getConfig().pluginProxyUrl,
      clusterId,
      pluginName: plugin.name,
      pluginVersion: plugin.installationVersion,
      path,
    });
  });

  protected installationId = computed(() => this.plugin()?.installationId ?? '');

  protected installationVersion = computed(() => this.plugin()?.installationVersion ?? '');

  errorMessage = signal<string | null>(null);

  crdDef = signal<ParsedCrd | undefined>(undefined);

  namespaces = signal<string[]>([]);

  private crdLoaded = signal(false);

  private namespacesReady = signal(false);

  // True until the cluster list, CRD, and (project) namespaces have all settled.
  // Gating the iframe on this prevents two problems: flashing the "not available"
  // card before the CRD loads, and mounting the iframe before the namespace list
  // arrives — the plugin SDK's init is one-shot, so a late namespace update would
  // be lost and the form would render its free-text namespace fallback for good.
  protected loading = computed(() => {
    if (this.clusterContext.isLoadingClusters()) return true;
    // No cluster selected (none available or load failed): nothing left to wait for.
    if (this.clusterContext.selectedClusterId() === null) return false;
    return !this.crdLoaded() || !this.namespacesReady();
  });

  readonly listLink = ['..'];

  constructor() {
    this.titleService.setTitle('Create');

    // The effect fires when selectedClusterId is set by loadClusters() in ngOnInit.
    effect(() => {
      const clusterId = this.clusterContext.selectedClusterId();
      if (clusterId !== null) {
        untracked(() => this.loadCrd(clusterId));
      }
    });

    effect(() => {
      const projectId = this.projectId();
      untracked(() => {
        if (projectId) {
          this.loadNamespaces(projectId);
        } else {
          // Org-level route has no project to scope namespaces to; the form uses
          // its free-text namespace field. Nothing to load, so mark it settled.
          this.namespacesReady.set(true);
        }
      });
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      // Sets selectedClusterId on completion, triggering the effect above.
      await this.clusterContext.loadClusters();
    } catch {
      this.errorMessage.set('Failed to load clusters.');
    }
  }

  private async loadCrd(clusterId: string): Promise<void> {
    this.crdLoaded.set(false);
    this.errorMessage.set(null);

    try {
      // The create form needs only the CRD schema, not the resource list.
      const crd = await this.loader.loadCrd(this.pluginName(), this.resourceKind(), clusterId);
      this.crdDef.set(crd);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ResourceCreate] Failed to load resource definition:', err);
      this.errorMessage.set('Failed to load resource definition. Please try again.');
    } finally {
      this.crdLoaded.set(true);
    }
  }

  private async loadNamespaces(projectId: string): Promise<void> {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.namespaceClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces.map((n) => n.name));
    } catch (err) {
      // Transient failure: the plugin form falls back to a free-text namespace
      // field when the list is empty.
      // eslint-disable-next-line no-console
      console.error('[ResourceCreate] Failed to load namespaces:', err);
    } finally {
      this.namespacesReady.set(true);
    }
  }
}
