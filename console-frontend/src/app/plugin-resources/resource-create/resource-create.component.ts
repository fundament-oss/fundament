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
      kubeApiProxyUrl: this.config.getConfig().kubeApiProxyUrl,
      clusterId,
      pluginName: plugin.name,
      path,
    });
  });

  protected allowedResources = computed(() => this.plugin()?.allowedResources ?? []);

  isLoading = signal(false);

  errorMessage = signal<string | null>(null);

  crdDef = signal<ParsedCrd | undefined>(undefined);

  namespaces = signal<string[]>([]);

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
      if (projectId) {
        untracked(() => this.loadNamespaces(projectId));
      }
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
    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const { crd } = await this.loader.loadCrdAndResources(
        this.pluginName(),
        this.resourceKind(),
        clusterId,
      );
      this.crdDef.set(crd);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ResourceCreate] Failed to load resource definition:', err);
      this.errorMessage.set('Failed to load resource definition. Please try again.');
    } finally {
      this.isLoading.set(false);
    }
  }

  private async loadNamespaces(projectId: string): Promise<void> {
    try {
      const request = create(ListProjectNamespacesRequestSchema, { projectId });
      const response = await firstValueFrom(this.namespaceClient.listProjectNamespaces(request));
      this.namespaces.set(response.namespaces.map((n) => n.name));
    } catch (err) {
      // No project context (org-level) or transient failure: the plugin form
      // falls back to a free-text namespace field when the list is empty.
      // eslint-disable-next-line no-console
      console.error('[ResourceCreate] Failed to load namespaces:', err);
    }
  }
}
