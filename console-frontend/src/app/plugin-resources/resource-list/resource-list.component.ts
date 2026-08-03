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
import PluginIframeComponent from '../iframe/plugin-iframe.component';
import ResourceDeleteModalComponent from '../resource-delete-modal/resource-delete-modal.component';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import PluginRegistryService from '../plugin-registry.service';
import { deleteErrorMessage } from '../kube-api-error';
import { TitleService } from '../../title.service';
import { ConfigService } from '../../config.service';
import type { ParsedCrd, AdditionalPrinterColumn, KubeResource } from '../types';
import { buildCustomUIUrl } from '../plugin-console-url.utils';
import {
  resolveJsonPath,
  formatColumnValue,
  getListColumns,
  kindToLabel,
} from '../crd-schema.utils';

function buildDetailLink(resource: KubeResource): string[] {
  return ['.', resource.metadata.name];
}

function buildDetailQueryParams(resource: KubeResource): { ns: string } | null {
  return resource.metadata.namespace ? { ns: resource.metadata.namespace } : null;
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
  imports: [RouterLink, PluginIframeComponent, ResourceDeleteModalComponent],
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

  private config = inject(ConfigService);

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

  customUIUrl = computed(() =>
    buildCustomUIUrl({
      plugin: this.plugin(),
      kind: this.crdDef()?.kind,
      view: 'list',
      clusterId: this.clusterContext.selectedClusterId(),
      pluginProxyUrl: this.config.getConfig().pluginProxyUrl,
    }),
  );

  installationId = computed(() => this.plugin()?.installationId ?? '');

  installationVersion = computed(() => this.plugin()?.installationVersion ?? '');

  canCreate = computed(() => {
    const kind = this.crdDef()?.kind;
    if (!kind) return false;
    return Boolean(this.plugin()?.customComponents?.[kind]?.create);
  });

  readonly createLink = ['create'];

  columns = computed<AdditionalPrinterColumn[]>(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    return getListColumns(crd.additionalPrinterColumns).filter(
      (col) => col.name !== 'Name' && col.name !== 'Age',
    );
  });

  /**
   * `columns` track list for `nldd-table`. The plugin declares its own columns,
   * so the grid template has to be derived rather than written out literally.
   */
  tableColumns = computed(() =>
    ['minmax(200px, 1fr)', ...this.columns().map(() => 'minmax(120px, 1fr)'), '64px'].join(' '),
  );

  /** Same, minus the low-priority columns that `hide-below="lg"` drops. */
  tableMdColumns = computed(() =>
    [
      'minmax(200px, 1fr)',
      ...this.columns()
        .filter((col) => !col.priority || col.priority === 0)
        .map(() => 'minmax(120px, 1fr)'),
      '64px',
    ].join(' '),
  );

  kindLabel = computed(() => {
    const crd = this.crdDef();
    if (crd) return kindToLabel(crd.kind);

    const plugin = this.plugin();
    const resourceKind = this.resourceKind();
    const allItems = [...(plugin?.menu.project ?? [])];
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

  detailQueryParams = buildDetailQueryParams;

  formatCell = buildCellValue;

  // --- Per-row delete ---

  pendingDelete = signal<KubeResource | null>(null);

  deleting = signal(false);

  deleteError = signal<string | null>(null);

  openDelete(resource: KubeResource): void {
    this.deleteError.set(null);
    this.pendingDelete.set(resource);
  }

  closeDelete(): void {
    if (!this.deleting()) this.pendingDelete.set(null);
  }

  async confirmDelete(): Promise<void> {
    const crd = this.crdDef();
    const clusterId = this.clusterContext.selectedClusterId();
    const target = this.pendingDelete();
    if (!crd || !clusterId || !target) return;

    this.deleting.set(true);
    this.deleteError.set(null);
    try {
      await this.loader.deleteResource(
        this.pluginName(),
        crd,
        clusterId,
        target.metadata.name,
        target.metadata.namespace,
      );
      this.pendingDelete.set(null);
      await this.reloadResources(crd, clusterId);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ResourceList] Failed to delete resource:', err);
      this.deleteError.set(deleteErrorMessage(err));
    } finally {
      this.deleting.set(false);
    }
  }

  // Refreshes the list after a delete. The CRD schema and plugin registry are
  // unchanged, so re-list resources only rather than re-running the full
  // loadCrdsAndResources (which re-fetches the CRD schema).
  private async reloadResources(crd: ParsedCrd, clusterId: string): Promise<void> {
    try {
      this.resources.set(await this.loader.loadResources(this.pluginName(), crd, clusterId));
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ResourceList] Failed to reload resources:', err);
      this.errorMessage.set('Failed to load resources. Please try again.');
    }
  }
}
