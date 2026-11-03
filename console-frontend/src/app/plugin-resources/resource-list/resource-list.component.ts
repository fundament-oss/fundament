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
import pollResources from '../resource-poll';
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
  crdRefToLabel,
  kindToLabel,
  kindToSingularLabel,
  labelMidSentence,
} from '../crd-schema.utils';
import opensElsewhere from '../../opens-elsewhere';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/badge';
import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/card';
import '@nldd/design-system/collection';
import '@nldd/design-system/container';
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
  imports: [RouterLink, PluginIframeComponent, PluginStatusBannerComponent],
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

  private projectNamespaces = inject(ProjectNamespacesService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  protected pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  /** '' on the organization-level route, which has no project. */
  private projectId = computed(() => this.routeParams().get('id') ?? '');

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

  // The project's namespaces, forwarded to the plugin iframe for the create
  // sheet a custom list opens, which gets them from the list as the routed
  // create page gets them from its own route. Cluster-side names
  // ("tnt-<project>--<name>"): the plugin passes them to the Kubernetes API as is.
  namespaces = signal<string[]>([]);

  // The project-level name for each of namespaces, which the plugin shows.
  namespaceDisplayNames = signal<Record<string, string>>({});

  // Holds the iframe back until the namespace lookup has finished, failed or
  // not: the iframe sends them once, in fundament:init, and never again.
  namespacesSettled = signal(false);

  // Counts namespace loads, so a response for a project the route has since
  // moved away from is dropped.
  private namespacesRequest = 0;

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
    const projectId = this.route.snapshot.parent?.params['id'];
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

  private menuItem = computed(() =>
    this.plugin()?.menu.project?.find((i) => i.crd === this.resourceKind()),
  );

  /** The plugin's own menu label wins, so the page title matches the nav item
   *  (CloudNativePG calls its Clusters "Databases"). */
  kindLabel = computed(() => {
    const label = this.menuItem()?.label;
    if (label) return label;

    const crd = this.crdDef();
    if (crd) return kindToLabel(crd.kind);

    // The route param is a CRD reference ("certificates.cert-manager.io"), not a
    // kind, so it must not go through kindToLabel — that would pluralize the group.
    return crdRefToLabel(this.resourceKind());
  });

  /** The list's name as it reads inside a sentence ("No DNS endpoints found").
   *  A plain toLowerCase() on the label flattened the acronym too: "no dns
   *  endpoints found". */
  kindLabelMidSentence = computed(() => labelMidSentence(this.kindLabel()));

  /** One item, for the create button: "New database…", not "New databases…".
   *  Only shown once the CRD is loaded, so the kind is always there to fall back on. */
  singularKindLabelMidSentence = computed(() =>
    labelMidSentence(
      this.menuItem()?.singularLabel ?? kindToSingularLabel(this.crdDef()?.kind ?? ''),
    ),
  );

  constructor() {
    effect(() => {
      this.titleService.setTitle(this.kindLabel());
    });

    // Another project's list reuses this component, so follow the route.
    effect(() => {
      const projectId = this.projectId();
      untracked(() => this.loadNamespaces(projectId));
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

    pollResources(inject(DestroyRef), () => this.refreshResources());
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

  /** Unmounts the iframe until the project's namespaces are in, so a page
   *  initialised with the previous project's namespaces does not keep them. */
  private async loadNamespaces(projectId: string): Promise<void> {
    this.namespacesRequest += 1;
    const current = this.namespacesRequest;
    this.namespacesSettled.set(false);
    this.namespaces.set([]);
    this.namespaceDisplayNames.set({});
    if (!projectId) {
      // The organization-level route has no project to scope namespaces to.
      this.namespacesSettled.set(true);
      return;
    }
    try {
      const { names, displayNames } = await this.projectNamespaces.list(projectId);
      if (current !== this.namespacesRequest) return;
      this.namespaces.set(names);
      this.namespaceDisplayNames.set(displayNames);
    } catch (err) {
      if (current !== this.namespacesRequest) return;
      // Transient failure: plugin UIs fall back to a free-text namespace field.
      // eslint-disable-next-line no-console
      console.error('[ResourceList] Failed to load namespaces:', err);
    } finally {
      if (current === this.namespacesRequest) this.namespacesSettled.set(true);
    }
  }

  /**
   * The poll's refresh, so a status column follows the cluster. Re-lists the
   * resources only: the CRD schema is unchanged, so this skips the full
   * loadCrdsAndResources and its loading state, which would blank the list
   * every few seconds. Only for the generated list; a custom list UI reads its
   * own resources. A failed poll keeps what is shown, and the next one tries
   * again.
   */
  private async refreshResources(): Promise<void> {
    const crd = this.crdDef();
    const clusterId = this.clusterContext.selectedClusterId();
    if (!crd || clusterId === null || this.isLoading() || this.customUIUrl()) return;
    try {
      const resources = await this.loader.loadResources(this.pluginName(), crd, clusterId);
      // Moved on to another kind or cluster while this was out.
      if (this.crdDef() !== crd || this.clusterContext.selectedClusterId() !== clusterId) return;
      this.resources.set(resources);
    } catch {
      // Kept as is; see above.
    }
  }
}
