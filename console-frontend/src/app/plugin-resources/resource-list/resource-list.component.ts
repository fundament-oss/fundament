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
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import PluginIframeComponent from '../iframe/plugin-iframe.component';
import ResourceDeleteModalComponent from '../resource-delete-modal/resource-delete-modal.component';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import PluginRegistryService from '../plugin-registry.service';
import { deleteErrorMessage } from '../kube-api-error';
import { TitleService } from '../../title.service';
import PageNavService from '../../page-nav.service';
import { ConfigService } from '../../config.service';
import type { ParsedCrd, AdditionalPrinterColumn, KubeResource } from '../types';
import { buildCustomUIUrl } from '../plugin-console-url.utils';
import { isDroppedWhenNarrow, narrowTableTracks, tableTracks } from '../table-tracks';
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
  protected pageNav = inject(PageNavService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

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

  /** Where the back button leads: the project these resources belong to, or the
   *  organization when the plugin is installed there rather than in a project. */
  backPath = computed(() => {
    const projectId = this.route.snapshot.parent?.parent?.params['id'];
    return projectId ? `/projects/${projectId}` : '/';
  });

  columns = computed<AdditionalPrinterColumn[]>(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    return getListColumns(crd.additionalPrinterColumns).filter(
      (col) => col.name !== 'Name' && col.name !== 'Age',
    );
  });

  tableColumns = computed(() => tableTracks(this.columns()));

  tableMdColumns = computed(() => narrowTableTracks(this.columns()));

  protected readonly isDroppedWhenNarrow = isDroppedWhenNarrow;

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

  /** Resolved URL of a resource's detail page, for the `href` of a menu item. */
  protected detailHref(resource: KubeResource): string {
    return this.router.serializeUrl(
      this.router.createUrlTree(buildDetailLink(resource), {
        relativeTo: this.route,
        queryParams: buildDetailQueryParams(resource),
      }),
    );
  }

  /**
   * Routes a left-click on the "View" menu item in-app.
   *
   * `nldd-menu-item[href]` renders a real anchor, which is what gives the item its
   * middle-click and "open in new tab" behaviour — but that anchor sits in the
   * element's shadow DOM, so `routerLink` cannot bind to it and the browser would
   * do a full page load. Anything with a modifier is left to the browser.
   */
  protected onDetailClick(event: MouseEvent, resource: KubeResource): void {
    if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    this.router.navigate(buildDetailLink(resource), {
      relativeTo: this.route,
      queryParams: buildDetailQueryParams(resource),
    });
  }

  formatCell = buildCellValue;

  /** The first column that reads as a condition carries the card's badge, the
   *  way a cluster's status does; the rest become rows in the card's list. */
  conditionColumn = computed(() =>
    this.columns().find((col) =>
      this.resources().some((resource) => this.conditionBadge(col.name, buildCellValue(resource, col))),
    ),
  );

  detailColumns = computed(() => this.columns().filter((col) => col !== this.conditionColumn()));

  /** A printer column that says True or False is a condition the platform keeps,
   *  not a word: it gets a badge, the same as a cluster's status. The column
   *  names it, so "Ready" reads as Ready and Not ready rather than as True and
   *  False, and a column called something else follows along. Anything that is
   *  not exactly True or False stays text, whatever the plugin puts there. */
  // eslint-disable-next-line class-methods-use-this
  conditionBadge(column: string, value: string): { text: string; color: string } | null {
    if (value === 'True') return { text: column, color: 'success' };
    if (value === 'False') {
      return { text: `Not ${column.charAt(0).toLowerCase()}${column.slice(1)}`, color: 'warning' };
    }
    return null;
  }

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
