import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  effect,
  untracked,
  DestroyRef,
  OnInit,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import PluginIframeComponent from '../iframe/plugin-iframe.component';
import PluginStatusBannerComponent from '../plugin-status-banner.component';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import PluginRegistryService from '../plugin-registry.service';
import ProjectNamespacesService from '../project-namespaces.service';
import ProjectNamespacesLoader from '../project-namespaces-loader';
import pollResources from '../resource-poll';
import type { ListScope } from '../plugin-resource-store.service';
import DropdownSyncDirective from '../../dropdown-sync.directive';
import pluginRouteScope from '../plugin-route-scope';
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
  labelMidSentence,
  resourceListLabel,
  resourceSingularLabelMidSentence,
} from '../crd-schema.utils';
import opensElsewhere from '../../opens-elsewhere';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/badge';
import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/card';
import '@nldd/design-system/collection';
import '@nldd/design-system/container';
import '@nldd/design-system/dropdown';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-title-bar';

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
  imports: [RouterLink, PluginIframeComponent, PluginStatusBannerComponent, DropdownSyncDirective],
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

  /** Undefined on a cluster page; the plugin UI then lists cluster-wide. */
  protected projectId = computed(() => this.scope().projectId ?? undefined);

  private scope = computed(() =>
    pluginRouteScope(this.routeParams().get('id') ?? '', this.route.snapshot.data),
  );

  /** The route's cluster on a cluster page; on a project page, the project's
   *  cluster as the registry follows it. */
  protected clusterId = computed(
    () => this.scope().clusterId ?? this.clusterContext.selectedClusterId(),
  );

  protected loadingClusters = computed(
    () => this.scope().clusterId === null && this.clusterContext.isLoadingClusters(),
  );

  private queryParams = toSignal(this.route.queryParamMap, {
    initialValue: this.route.snapshot.queryParamMap,
  });

  /** The picker only applies to a namespaced kind on a project page. */
  protected showNamespacePicker = computed(
    () =>
      this.scope().projectId !== null &&
      this.crdDef()?.scope === 'Namespaced' &&
      this.namespaces().length > 0,
  );

  /** The ?namespace= choice; one outside the project counts as no choice. */
  protected selectedNamespace = computed(() => {
    const ns = this.queryParams().get('namespace');
    return ns && this.namespaces().includes(ns) ? ns : '';
  });

  /** Narrows the list to the project, or to the namespace picked; none on a cluster page. */
  private listScope = computed<ListScope | undefined>(() => {
    const { projectId } = this.scope();
    if (projectId === null) return undefined;
    const namespace = this.selectedNamespace();
    return namespace ? { projectId, namespace } : { projectId };
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
      clusterId: this.clusterId(),
      pluginProxyUrl: this.config.getConfig().pluginProxyUrl,
    }),
  );

  // The project's namespaces, forwarded to the plugin iframe for the create
  // sheet a custom list opens, which gets them from the list as the routed
  // create page gets them from its own route. The iframe is held back until
  // they have settled.
  private projectNamespaces = new ProjectNamespacesLoader(
    inject(ProjectNamespacesService),
    'ResourceList',
  );

  namespaces = this.projectNamespaces.names;

  namespaceDisplayNames = this.projectNamespaces.displayNames;

  namespacesSettled = this.projectNamespaces.settled;

  installationId = computed(() => this.plugin()?.installationId ?? '');

  installationVersion = computed(() => this.plugin()?.installationVersion ?? '');

  canCreate = computed(() => {
    const kind = this.crdDef()?.kind;
    if (!kind) return false;
    return Boolean(this.plugin()?.customComponents?.[kind]?.create);
  });

  readonly createLink = ['create'];

  /** Where the back button leads: the cluster or project page these resources
   *  are listed on. */
  backPath = computed(() => this.scope().backPath);

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

  private menuItem = computed(() =>
    this.plugin()?.menu[this.scope().menuSection]?.find((i) => i.crd === this.resourceKind()),
  );

  /** The plugin's own menu label wins, so the page title matches the nav item. */
  kindLabel = computed(() =>
    resourceListLabel(this.menuItem(), this.crdDef()?.kind, this.resourceKind()),
  );

  /** The list's name as it reads inside a sentence ("No DNS endpoints found").
   *  A plain toLowerCase() on the label flattened the acronym too: "no dns
   *  endpoints found". */
  kindLabelMidSentence = computed(() => labelMidSentence(this.kindLabel()));

  /** One item, for the create button: "New database…", not "New databases…".
   *  Only shown once the CRD is loaded, so the kind is always there to fall back on. */
  singularKindLabelMidSentence = computed(() =>
    resourceSingularLabelMidSentence(this.menuItem(), this.crdDef()?.kind),
  );

  // The latest load wins: results and loading state of a superseded one are dropped.
  private loadToken = 0;

  private loadedKey = '';

  constructor() {
    effect(() => {
      this.titleService.setTitle(this.kindLabel());
    });

    // Another project's list reuses this component, so follow the route.
    effect(() => {
      const projectId = this.scope().projectId ?? '';
      untracked(() => this.projectNamespaces.load(projectId));
    });

    // On a project page the effect fires when selectedClusterId is set by
    // loadClusters() in ngOnInit.
    effect(() => {
      const pluginName = this.pluginName();
      const resourceKind = this.resourceKind();
      const clusterId = this.clusterId();
      const listScope = this.listScope();
      if (pluginName && resourceKind && clusterId !== null && this.namespacesSettled()) {
        untracked(() => {
          const loadedKey = `${pluginName}/${resourceKind}/${clusterId}`;
          const crd = this.crdDef();
          // Only the namespace changed: re-list without resetting the CRD, so the
          // picker stays mounted and keeps focus.
          if (crd && loadedKey === this.loadedKey) {
            this.reloadResources(crd, clusterId, listScope);
          } else {
            this.loadedKey = loadedKey;
            this.loadCrdsAndResources(pluginName, resourceKind, clusterId, listScope);
          }
        });
      }
    });

    pollResources(inject(DestroyRef), () => this.refreshResources());
  }

  async ngOnInit(): Promise<void> {
    // A cluster route names its cluster; only a project route needs the list.
    if (this.scope().clusterId !== null) return;
    try {
      // Sets selectedClusterId on completion, triggering the effect above.
      await this.clusterContext.loadClusters();
    } catch {
      this.errorMessage.set('Failed to load clusters.');
    }
  }

  protected onNamespaceChange(event: Event): void {
    const value = (event.target as HTMLSelectElement).value;
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { namespace: value || null },
      queryParamsHandling: 'merge',
    });
  }

  private async loadCrdsAndResources(
    pluginName: string,
    resourceKind: string,
    clusterId: string,
    listScope: ListScope | undefined,
  ): Promise<void> {
    this.loadToken += 1;
    const token = this.loadToken;
    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.crdDef.set(undefined);
    this.resources.set([]);

    try {
      const { crd, resources } = await this.loader.loadCrdAndResources(
        pluginName,
        resourceKind,
        clusterId,
        listScope,
      );
      if (token !== this.loadToken) return;
      this.crdDef.set(crd);
      this.resources.set(resources);
    } catch (err) {
      if (token !== this.loadToken) return;
      // eslint-disable-next-line no-console
      console.error('[ResourceList] Failed to load resources:', err);
      this.errorMessage.set('Failed to load resources. Please try again.');
    } finally {
      if (token === this.loadToken) this.isLoading.set(false);
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
    if (opensElsewhere(event)) return;
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
      this.resources().some((resource) =>
        this.conditionBadge(col.name, buildCellValue(resource, col)),
      ),
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

  /**
   * The poll's refresh, so a status column follows the cluster. Re-lists the
   * resources only: the CRD schema is unchanged, so this skips the full
   * loadCrdsAndResources and its loading state, which would blank the list
   * every few seconds. Only for the generated list; a custom list UI reads its
   * own resources. A failed poll keeps what is shown, and the next one tries
   * again. Until the CRD has been read it retries the whole load instead: one
   * that failed, or found the CRD not served yet (a plugin still deploying, or
   * Failed), would otherwise stay empty after the plugin recovers.
   */
  private async refreshResources(): Promise<void> {
    const crd = this.crdDef();
    const clusterId = this.clusterId();
    const listScope = this.listScope();
    if (clusterId === null || this.isLoading()) return;
    if (!crd) {
      await this.retryLoad(clusterId);
      return;
    }
    if (this.customUIUrl()) return;
    try {
      const resources = await this.loader.loadResources(
        this.pluginName(),
        crd,
        clusterId,
        listScope,
      );
      // Moved on to another kind or cluster while this was out.
      if (
        this.crdDef() !== crd ||
        this.clusterId() !== clusterId ||
        this.listScope() !== listScope
      ) {
        return;
      }
      this.resources.set(resources);
    } catch {
      // Kept as is; see above.
    }
  }

  /** loadCrdsAndResources without its loading state, which would put a spinner
   *  over the error every few seconds. Clears the error once the load works. */
  private async retryLoad(clusterId: string): Promise<void> {
    const pluginName = this.pluginName();
    const resourceKind = this.resourceKind();
    const listScope = this.listScope();
    if (!pluginName || !resourceKind) return;
    try {
      const { crd, resources } = await this.loader.loadCrdAndResources(
        pluginName,
        resourceKind,
        clusterId,
        listScope,
      );
      // Moved on, or a full load started, while this was out.
      if (
        !crd ||
        this.crdDef() !== undefined ||
        this.isLoading() ||
        this.pluginName() !== pluginName ||
        this.resourceKind() !== resourceKind ||
        this.clusterId() !== clusterId ||
        this.listScope() !== listScope
      ) {
        return;
      }
      this.crdDef.set(crd);
      this.resources.set(resources);
      this.errorMessage.set(null);
    } catch {
      // Kept as is; see above.
    }
  }

  // Re-lists resources only (after a namespace change); the CRD schema and
  // plugin registry are unchanged, so the schema is not re-fetched.
  private async reloadResources(
    crd: ParsedCrd,
    clusterId: string,
    listScope: ListScope | undefined,
  ): Promise<void> {
    this.loadToken += 1;
    const token = this.loadToken;
    this.isLoading.set(true);
    this.errorMessage.set(null);
    try {
      const resources = await this.loader.loadResources(
        this.pluginName(),
        crd,
        clusterId,
        listScope,
      );
      if (token === this.loadToken) this.resources.set(resources);
    } catch (err) {
      if (token !== this.loadToken) return;
      // eslint-disable-next-line no-console
      console.error('[ResourceList] Failed to reload resources:', err);
      this.errorMessage.set('Failed to load resources. Please try again.');
    } finally {
      if (token === this.loadToken) this.isLoading.set(false);
    }
  }
}
