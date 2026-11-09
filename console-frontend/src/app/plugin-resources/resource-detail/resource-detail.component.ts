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
import FieldRendererComponent from '../field-renderers/field-renderer.component';
import PluginIframeComponent from '../iframe/plugin-iframe.component';
import PluginStatusBannerComponent from '../plugin-status-banner.component';
import ResourceDeleteModalComponent from '../resource-delete-modal/resource-delete-modal.component';
import PluginRegistryService from '../plugin-registry.service';
import { deleteErrorMessage } from '../kube-api-error';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import pollResources from '../resource-poll';
import { TitleService } from '../../title.service';
import PageNavService from '../../page-nav.service';
import { ConfigService } from '../../config.service';
import ProjectNamespacesService from '../project-namespaces.service';
import ProjectNamespacesLoader from '../project-namespaces-loader';
import type { ParsedCrd, KubeResource, CrdPropertySchema } from '../types';
import { toDateValue, toSimpleValue, fieldNameToLabel, kindToLabel } from '../crd-schema.utils';
import { buildCustomUIUrl } from '../plugin-console-url.utils';
import opensElsewhere from '../../opens-elsewhere';

import '@nldd/design-system/badge';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/container';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';

/** The resource a detail route names. */
interface ResourceTarget {
  pluginName: string;
  resourceKind: string;
  name: string;
  namespace: string | undefined;
}

function checkIsConditionsField(key: string, value: unknown): boolean {
  return key === 'conditions' && Array.isArray(value);
}

function toArray(val: unknown): unknown[] {
  return Array.isArray(val) ? val : [];
}

function toRecord(val: unknown): Record<string, unknown> {
  return (val as Record<string, unknown>) ?? {};
}

@Component({
  selector: 'app-resource-detail',
  imports: [
    RouterLink,
    FieldRendererComponent,
    PluginIframeComponent,
    PluginStatusBannerComponent,
    ResourceDeleteModalComponent,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './resource-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceDetailComponent implements OnInit {
  protected pageNav = inject(PageNavService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private titleService = inject(TitleService);

  private registry = inject(PluginRegistryService);

  protected clusterContext = inject(KubeClusterContextService);

  private loader = inject(KubePluginLoaderService);

  private config = inject(ConfigService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private routeQuery = toSignal(this.route.queryParamMap, {
    initialValue: this.route.snapshot.queryParamMap,
  });

  protected pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  protected resourceId = computed(() => this.routeParams().get('resourceId') ?? '');

  protected resourceNamespace = computed(() => this.routeQuery().get('ns') || undefined);

  /** The resource the route names, as one value: the load follows it, and the
   *  plugin's frame is keyed on it. */
  private resourceTarget = computed<ResourceTarget>(
    () => ({
      pluginName: this.pluginName(),
      resourceKind: this.resourceKind(),
      name: this.resourceId(),
      namespace: this.resourceNamespace(),
    }),
    {
      equal: (a, b) =>
        a.pluginName === b.pluginName &&
        a.resourceKind === b.resourceKind &&
        a.name === b.name &&
        a.namespace === b.namespace,
    },
  );

  /** One key per resource the route names. The plugin's frame gets the
   *  resource once, in fundament:init, so another resource needs a new frame. */
  protected resourceKey = computed(() => {
    const target = this.resourceTarget();
    return `${target.pluginName}/${target.resourceKind}/${target.namespace ?? ''}/${target.name}`;
  });

  // Bumped by every full load, so a read that one overtook can tell its result
  // is for a resource no longer shown and drop it.
  private loads = 0;

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  customUIUrl = computed(() =>
    buildCustomUIUrl({
      plugin: this.plugin(),
      kind: this.crdDef()?.kind,
      view: 'detail',
      clusterId: this.clusterContext.selectedClusterId(),
      pluginProxyUrl: this.config.getConfig().pluginProxyUrl,
    }),
  );

  protected installationId = computed(() => this.plugin()?.installationId ?? '');

  protected installationVersion = computed(() => this.plugin()?.installationVersion ?? '');

  isLoading = signal(false);

  errorMessage = signal<string | null>(null);

  crdDef = signal<ParsedCrd | undefined>(undefined);

  resource = signal<KubeResource | undefined>(undefined);

  // The CRD is the authoritative source for kind/apiVersion: a resource resolved
  // via the list-and-match fallback (deep link without ?ns=) is a List item,
  // which the apiserver returns without kind/apiVersion set.
  kind = computed(() => this.crdDef()?.kind ?? this.resource()?.kind ?? '');

  apiVersion = computed(() => {
    const crd = this.crdDef();
    if (crd) return `${crd.group}/${crd.version}`;
    return this.resource()?.apiVersion ?? '';
  });

  specSections = computed(() => {
    const crd = this.crdDef();
    if (!crd) return [];
    const fields = Object.entries(crd.specSchema.properties) as [string, CrdPropertySchema][];
    return [{ name: 'Configuration', fields }];
  });

  statusFields = computed<[string, unknown][]>(() => {
    const r = this.resource();
    if (!r?.status) return [];
    return Object.entries(r.status);
  });

  constructor() {
    effect(() => {
      const r = this.resource();
      this.titleService.setTitle(r?.metadata.name);
    });

    this.projectNamespaces.load(this.route.snapshot.parent?.params['id'] ?? '');

    // The effect fires when selectedClusterId is set by loadClusters() in ngOnInit.
    // It follows the route too: another resource's detail reuses this
    // component (a sheet that navigates to the resource it just created).
    effect(() => {
      const clusterId = this.clusterContext.selectedClusterId();
      const target = this.resourceTarget();
      if (clusterId !== null) {
        untracked(() => this.loadCrdAndResource(clusterId, target));
      }
    });

    pollResources(inject(DestroyRef), () => this.refreshResource());
  }

  async ngOnInit(): Promise<void> {
    try {
      // Sets selectedClusterId on completion, triggering the effect above.
      await this.clusterContext.loadClusters();
    } catch {
      this.errorMessage.set('Failed to load clusters.');
    }
  }

  private async loadCrdAndResource(clusterId: string, target: ResourceTarget): Promise<void> {
    this.loads += 1;
    const load = this.loads;
    this.isLoading.set(true);
    this.errorMessage.set(null);
    // What was shown is another resource's, or another cluster's. The CRD
    // too: the route may name another kind, whose schema and custom page
    // the previous one's must not stand in for, nor be polled with should
    // this load fail.
    this.crdDef.set(undefined);
    this.resource.set(undefined);

    try {
      const { crd, resource } = await this.loader.loadCrdAndResource(
        target.pluginName,
        target.resourceKind,
        clusterId,
        target.name,
        target.namespace,
      );
      if (load !== this.loads) return;
      this.crdDef.set(crd);
      this.resource.set(resource);
    } catch (err) {
      if (load !== this.loads) return;
      // eslint-disable-next-line no-console
      console.error('[ResourceDetail] Failed to load resource:', err);
      this.errorMessage.set('Failed to load resource. Please try again.');
    } finally {
      if (load === this.loads) this.isLoading.set(false);
    }
  }

  /**
   * The poll's refresh, so the status and conditions follow the cluster.
   * Re-reads the resource only, without the loading state, which would blank
   * the page every few seconds. Only for the generated view; a custom detail UI
   * reads its own. Read by the loaded resource's own name and namespace, as the
   * delete is. Held back while a delete is out: the resource vanishing under
   * the modal would swap the page for "not found" before the delete returns.
   * A resource that is gone does show as gone, and keeps being read by the
   * route's name: a 404 can be passing (a CRD not served for a moment during a
   * plugin upgrade), and the resource then shows again by itself. A failed poll
   * keeps what is shown, and the next one tries again. Until the CRD has been
   * read it retries the whole load instead: one that failed, or found the CRD
   * not served yet, would otherwise stay empty after the plugin recovers.
   */
  private async refreshResource(): Promise<void> {
    const crd = this.crdDef();
    const clusterId = this.clusterContext.selectedClusterId();
    const current = this.resource();
    const load = this.loads;
    if (clusterId === null || this.isLoading() || this.deleting()) return;
    if (!crd) {
      await this.retryLoad(clusterId);
      return;
    }
    if (this.customUIUrl()) return;
    try {
      const resource = await this.loader.loadResource(
        this.pluginName(),
        crd,
        clusterId,
        current?.metadata.name ?? this.resourceId(),
        current ? current.metadata.namespace : this.resourceNamespace(),
      );
      // Moved on to another resource or cluster, or deleting, while this was out.
      if (load !== this.loads || this.deleting()) return;
      if (this.resource()?.metadata.uid !== current?.metadata.uid) return;
      if (this.clusterContext.selectedClusterId() !== clusterId) return;
      this.resource.set(resource);
    } catch {
      // Kept as is; see above.
    }
  }

  /** loadCrdAndResource without its loading state, which would put a spinner
   *  over the error every few seconds. Clears the error once the load works. */
  private async retryLoad(clusterId: string): Promise<void> {
    const load = this.loads;
    const target = this.resourceTarget();
    try {
      const { crd, resource } = await this.loader.loadCrdAndResource(
        target.pluginName,
        target.resourceKind,
        clusterId,
        target.name,
        target.namespace,
      );
      // A full load started, for this resource or another, while this was out.
      if (!crd || load !== this.loads || this.crdDef() !== undefined) return;
      if (this.clusterContext.selectedClusterId() !== clusterId) return;
      this.crdDef.set(crd);
      this.resource.set(resource);
      this.errorMessage.set(null);
    } catch {
      // Kept as is; see refreshResource.
    }
  }

  readonly listLink = ['..'];

  /** The same destination as an href, so the back link is a real link: it can be
   *  middle-clicked and copied. routerLink cannot reach the anchor, which lives
   *  in nldd-link's shadow DOM. */
  get listHref(): string {
    return this.router.createUrlTree(this.listLink, { relativeTo: this.route }).toString();
  }

  /** Names the list the back button returns to: the plugin's menu label, else
   *  the CRD's kind, so the button says where it goes. Built from the kind, not
   *  its plural: the plural is lowercase by Kubernetes' rules, which capitalized
   *  gave "Dnsendpoints". */
  kindLabel = computed(() => {
    const resourceKind = this.resourceKind();
    const item = this.plugin()?.menu.project?.find((i) => i.crd === resourceKind);
    if (item?.label) return item.label;

    const kind = this.crdDef()?.kind;
    return kind ? kindToLabel(kind) : this.kind() || 'list';
  });

  /** Where the title bar's back button leads: the project this resource belongs
   *  to, or the organization when the plugin is installed there. The way back to
   *  the list is a link in the page itself. */
  backPath = computed(() => {
    const projectId = this.route.snapshot.parent?.params['id'];
    return projectId ? `/projects/${projectId}` : '/';
  });

  // The project's namespaces, forwarded to the plugin iframe: custom detail
  // UIs embed create flows (the ceph-rook Buckets section), which need the
  // same namespace choices the create view gets. The iframe is held back until
  // they have settled.
  private projectNamespaces = new ProjectNamespacesLoader(
    inject(ProjectNamespacesService),
    'ResourceDetail',
  );

  namespaces = this.projectNamespaces.names;

  namespaceDisplayNames = this.projectNamespaces.displayNames;

  namespacesSettled = this.projectNamespaces.settled;

  goToList(event?: Event) {
    // The back event bubbles and composes, so the split view in the shell hears
    // it too and would navigate one step further up on its own.
    event?.stopPropagation();
    this.router.navigate(this.listLink, { relativeTo: this.route });
  }

  onBackClick(event: Event) {
    if (opensElsewhere(event)) return;
    event.preventDefault();
    this.router.navigate(this.listLink, { relativeTo: this.route });
  }

  showDeleteModal = signal(false);

  deleting = signal(false);

  deleteError = signal<string | null>(null);

  // Only offer delete in the native detail view — custom-UI plugins manage their
  // own resources through their iframe.
  canDelete = computed(() => !this.customUIUrl() && this.resource() !== undefined);

  openDeleteModal(): void {
    this.deleteError.set(null);
    this.showDeleteModal.set(true);
  }

  closeDeleteModal(): void {
    if (!this.deleting()) this.showDeleteModal.set(false);
  }

  async confirmDelete(): Promise<void> {
    const crd = this.crdDef();
    const clusterId = this.clusterContext.selectedClusterId();
    const resource = this.resource();
    if (!crd || !clusterId || !resource) return;

    this.deleting.set(true);
    this.deleteError.set(null);
    try {
      // Delete by the loaded resource's own name/namespace, not the route's ?ns=
      // query param: a deep link without ?ns= still resolves the resource via the
      // list-and-match fallback, so metadata.namespace is the authoritative value.
      await this.loader.deleteResource(
        this.pluginName(),
        crd,
        clusterId,
        resource.metadata.name,
        resource.metadata.namespace,
      );
      this.showDeleteModal.set(false);
      this.router.navigate(this.listLink, { relativeTo: this.route });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ResourceDetail] Failed to delete resource:', err);
      this.deleteError.set(deleteErrorMessage(err));
    } finally {
      this.deleting.set(false);
    }
  }

  formatLabel = fieldNameToLabel;

  formatDateValue = toDateValue;

  formatSimpleValue = toSimpleValue;

  /**
   * A condition as one badge: the type and its status in the same breath, so a
   * row does not need a column that only ever says True or False. `status` is
   * True, False or Unknown, and the type names what it is about.
   */
  // eslint-disable-next-line class-methods-use-this
  conditionBadge(type: string, status: unknown): { text: string; color: string } {
    if (status === 'True') return { text: type, color: 'success' };
    if (status === 'False') {
      return { text: `Not ${type.charAt(0).toLowerCase()}${type.slice(1)}`, color: 'warning' };
    }
    return { text: `${type} unknown`, color: 'neutral' };
  }

  /** Every condition the resource reports, each with its own status, message and
   *  the moment it last changed. Usually one, but a controller is free to keep
   *  several apart, and then they each have their own history. */
  conditions = computed(() =>
    this.statusFields()
      .filter(([key, value]) => checkIsConditionsField(key, value))
      .flatMap(([, value]) => toArray(value))
      .map((entry) => toRecord(entry)),
  );

  /**
   * Better words for status fields whose own name reads like schema. The console
   * knows nothing about a plugin, so this stays a short list of names we can
   * improve on rather than a place to describe a plugin's whole vocabulary; a
   * field that is not in it keeps the name the CRD gave it.
   *
   * `notAfter` is the moment the certificate stops being valid, and
   * `renewalTime` the moment the platform renews it. That second one lies in
   * the future, hence "Renews on" and not "Renewed on".
   */
  private static readonly STATUS_LABELS: Record<string, string> = {
    notAfter: 'Valid until',
    notBefore: 'Valid from',
    renewalTime: 'Renews on',
  };

  // eslint-disable-next-line class-methods-use-this
  statusLabel(key: string): string {
    return ResourceDetailComponent.STATUS_LABELS[key] ?? fieldNameToLabel(key);
  }

  /** What the status reports outside its conditions: single values like a
   *  certificate's expiry. They read as facts about the resource, so they join
   *  the list at the top and leave the status block to the conditions. */
  flatStatusFields = computed(() =>
    this.statusFields().filter(([key, value]) => !checkIsConditionsField(key, value)),
  );

  /** The one that says whether the thing works, beside the title where a cluster
   *  carries its status too. Ready if there is one, otherwise the first. */
  primaryCondition = computed<{ text: string; color: string } | null>(() => {
    const all = this.conditions();
    const ready = all.find((entry) => entry['type'] === 'Ready') ?? all[0];
    return ready ? this.conditionBadge(String(ready['type'] ?? ''), ready['status']) : null;
  });

  isConditionsField = checkIsConditionsField;

  /**
   * An object whose values are all scalars reads as rows of its own: the field
   * name as a heading and one indented row per key, so the name is said once
   * instead of at the start of every line. Anything deeper stays with the field
   * renderer, which knows how to fold it into a single cell.
   */
  // eslint-disable-next-line class-methods-use-this
  flatObjectEntries(value: unknown): [string, unknown][] | null {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) return null;
    if (entries.some(([, entry]) => entry !== null && typeof entry === 'object')) return null;
    return entries;
  }

  /** A field off a condition as a plain string, for the template. */
  // eslint-disable-next-line class-methods-use-this
  asRecordValue(entry: Record<string, unknown>, key: string): string {
    const value = entry[key];
    return value == null ? '' : String(value);
  }

  asArray = toArray;

  asRecord = toRecord;

  getSpecValue(fieldName: string): unknown {
    return this.resource()?.spec?.[fieldName] ?? null;
  }
}
