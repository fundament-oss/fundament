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
import PluginStatusBannerComponent from '../plugin-status-banner.component';
import PluginRegistryService from '../plugin-registry.service';
import pluginRouteScope from '../plugin-route-scope';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import { TitleService } from '../../title.service';
import { ConfigService } from '../../config.service';
import PageNavService from '../../page-nav.service';
import ProjectNamespacesService from '../project-namespaces.service';
import ProjectNamespacesLoader from '../project-namespaces-loader';
import type { ParsedCrd } from '../types';
import { buildCustomUIUrl } from '../plugin-console-url.utils';
import { resourceListLabel, resourceSingularLabelMidSentence } from '../crd-schema.utils';

import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

@Component({
  selector: 'app-resource-create',
  imports: [RouterLink, PluginIframeComponent, PluginStatusBannerComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './resource-create.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceCreateComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private titleService = inject(TitleService);

  private registry = inject(PluginRegistryService);

  protected clusterContext = inject(KubeClusterContextService);

  private loader = inject(KubePluginLoaderService);

  private config = inject(ConfigService);

  protected pageNav = inject(PageNavService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private scope = computed(() =>
    pluginRouteScope(this.routeParams().get('id') ?? '', this.route.snapshot.data),
  );

  /** The route's cluster on a cluster page; on a project page, the project's
   *  cluster as the registry follows it. */
  protected clusterId = computed(
    () => this.scope().clusterId ?? this.clusterContext.selectedClusterId(),
  );

  protected pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  // Present only on the project-level route (projects/:id/...); empty on a cluster's.
  protected projectId = computed(() => this.scope().projectId ?? '');

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  private menuItem = computed(() =>
    this.plugin()?.menu[this.scope().menuSection]?.find((i) => i.crd === this.resourceKind()),
  );

  /** Names the list the back button returns to, as the list titles itself. */
  protected kindLabel = computed(() =>
    resourceListLabel(this.menuItem(), this.crdDef()?.kind, this.resourceKind()),
  );

  /** "New database", the same title a create sheet gets. Until the CRD has
   *  loaded there is no kind to name, and the title says what the page is for. */
  protected title = computed(() => {
    const singular = resourceSingularLabelMidSentence(this.menuItem(), this.crdDef()?.kind);
    return singular ? `New ${singular}` : 'New';
  });

  customUIUrl = computed(() =>
    buildCustomUIUrl({
      plugin: this.plugin(),
      kind: this.crdDef()?.kind,
      view: 'create',
      clusterId: this.clusterId(),
      pluginProxyUrl: this.config.getConfig().pluginProxyUrl,
    }),
  );

  protected installationId = computed(() => this.plugin()?.installationId ?? '');

  protected installationVersion = computed(() => this.plugin()?.installationVersion ?? '');

  errorMessage = signal<string | null>(null);

  crdDef = signal<ParsedCrd | undefined>(undefined);

  // The plugin form's free-text fallback would take the project-level name,
  // which is not the namespace's name on the cluster, so a failed load is said
  // on the page (namespacesUnavailable).
  private projectNamespaces = new ProjectNamespacesLoader(
    inject(ProjectNamespacesService),
    'ResourceCreate',
  );

  namespaces = this.projectNamespaces.names;

  namespaceDisplayNames = this.projectNamespaces.displayNames;

  private crdLoaded = signal(false);

  private namespacesReady = this.projectNamespaces.settled;

  protected namespacesFailed = this.projectNamespaces.failed;

  // Only a namespaced resource needs the list: a cluster-scoped create form
  // still works when it failed to load.
  protected namespacesUnavailable = computed(
    () => this.namespacesFailed() && this.crdDef()?.scope === 'Namespaced',
  );

  // A namespaced resource on a project route can only go in one of the project's
  // namespaces. With none there is nothing to pick, so the page points to where
  // they are made instead of mounting a form that cannot succeed.
  protected needsNamespace = computed(
    () =>
      !!this.projectId() &&
      this.crdDef()?.scope === 'Namespaced' &&
      !this.namespacesFailed() &&
      this.namespaces().length === 0,
  );

  protected namespacesLink = computed(() => `/projects/${this.projectId()}/namespaces`);

  // True until the cluster list, CRD, and (project) namespaces have all settled.
  // Gating the iframe on this prevents two problems: flashing the "not available"
  // card before the CRD loads, and mounting the iframe before the namespace list
  // arrives — the plugin SDK's init is one-shot, so a late namespace update would
  // be lost and the form would render its free-text namespace fallback for good.
  protected loading = computed(() => {
    if (this.clusterContext.isLoadingClusters()) return true;
    // No cluster selected (none available or load failed): nothing left to wait for.
    if (this.clusterId() === null) return false;
    return !this.crdLoaded() || !this.namespacesReady();
  });

  readonly listLink = ['..'];

  constructor() {
    effect(() => {
      this.titleService.setTitle(this.title());
    });

    // On a project page the effect fires when selectedClusterId is set by
    // loadClusters() in ngOnInit.
    effect(() => {
      const clusterId = this.clusterId();
      if (clusterId !== null) {
        untracked(() => this.loadCrd(clusterId));
      }
    });

    // A cluster's route has no project to scope namespaces to; the form then uses
    // its free-text namespace field.
    effect(() => {
      const projectId = this.projectId();
      untracked(() => this.projectNamespaces.load(projectId));
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

  goToList(event?: Event): void {
    // The back event bubbles and composes, so the split view in the shell hears
    // it too and would navigate one step further up on its own.
    event?.stopPropagation();
    this.router.navigate(this.listLink, { relativeTo: this.route });
  }
}
