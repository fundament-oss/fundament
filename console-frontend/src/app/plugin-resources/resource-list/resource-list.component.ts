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
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerDatabaseOff, tablerRefresh } from '@ng-icons/tabler-icons';
import PluginIframeComponent from '../iframe/plugin-iframe.component';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import PluginRegistryService from '../plugin-registry.service';
import { TitleService } from '../../title.service';
import type { ParsedCrd, AdditionalPrinterColumn, KubeResource } from '../types';
import {
  resolveJsonPath,
  formatColumnValue,
  getListColumns,
  kindToLabel,
} from '../crd-schema.utils';

function buildDetailLink(resource: KubeResource): string[] {
  return ['.', resource.metadata.name];
}

function buildCellValue(resource: KubeResource, col: AdditionalPrinterColumn): string {
  const fullObj = {
    metadata: resource.metadata,
    spec: resource.spec,
    status: resource.status ?? {},
  };
  const value = resolveJsonPath(fullObj, col.jsonPath);
  return formatColumnValue(value, col.type);
}

@Component({
  selector: 'app-resource-list',
  imports: [RouterLink, NgIcon, PluginIframeComponent],
  viewProviders: [provideIcons({ tablerDatabaseOff, tablerRefresh })],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './resource-list.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceListComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private registry = inject(PluginRegistryService);

  private titleService = inject(TitleService);

  protected clusterContext = inject(KubeClusterContextService);

  private loader = inject(KubePluginLoaderService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  protected pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  isLoading = signal(false);

  errorMessage = signal<string | null>(null);

  crdDef = signal<ParsedCrd | undefined>(undefined);

  resources = signal<KubeResource[]>([]);

  customUIUrl = computed(() => {
    const kind = this.crdDef()?.kind;
    if (!kind) return null;
    return this.plugin()?.customUI?.[kind]?.list ?? null;
  });

  columns = computed<AdditionalPrinterColumn[]>(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    return getListColumns(crd.additionalPrinterColumns).filter(
      (col) => col.name !== 'Name' && col.name !== 'Age',
    );
  });

  kindLabel = computed(() => {
    const crd = this.crdDef();
    if (crd) return kindToLabel(crd.kind);

    const plugin = this.plugin();
    const resourceKind = this.resourceKind();
    const allItems = [...(plugin?.menu.organization ?? []), ...(plugin?.menu.project ?? [])];
    const item = allItems.find((i) => i.crd === resourceKind);
    return item?.label ?? kindToLabel(resourceKind);
  });

  constructor() {
    effect(() => {
      this.titleService.setTitle(this.kindLabel());
    });

    // The effect fires when selectedClusterId is set by loadClusters() in ngOnInit.
    effect(() => {
      const pluginName = this.pluginName();
      const resourceKind = this.resourceKind();
      const clusterId = this.clusterContext.selectedClusterId();
      if (pluginName && resourceKind && clusterId !== null) {
        untracked(() => {
          this.loadCrdsAndResources(pluginName, resourceKind, clusterId);
        });
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

  onClusterChange(clusterId: string): void {
    this.clusterContext.onClusterChange(clusterId);
  }

  async onRefresh(): Promise<void> {
    const clusterId = this.clusterContext.selectedClusterId();
    const pluginName = this.pluginName();
    if (pluginName && this.resourceKind() && clusterId !== null) {
      await this.loadCrdsAndResources(pluginName, this.resourceKind(), clusterId);
    }
  }

  private async loadCrdsAndResources(
    pluginName: string,
    resourceKind: string,
    clusterId: string,
  ): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.crdDef.set(undefined);
    this.resources.set([]);

    try {
      const { crd, resources } = await this.loader.loadCrdAndResources(
        pluginName,
        resourceKind,
        clusterId,
      );
      this.crdDef.set(crd);
      this.resources.set(resources);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ResourceList] Failed to load resources:', err);
      this.errorMessage.set('Failed to load resources. Please try again.');
    } finally {
      this.isLoading.set(false);
    }
  }

  detailLink = buildDetailLink;

  formatCell = buildCellValue;
}
