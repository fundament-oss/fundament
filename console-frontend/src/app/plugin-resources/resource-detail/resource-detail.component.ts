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
import FieldRendererComponent from '../field-renderers/field-renderer.component';
import PluginIframeComponent from '../iframe/plugin-iframe.component';
import ResourceDeleteModalComponent from '../resource-delete-modal/resource-delete-modal.component';
import PluginRegistryService from '../plugin-registry.service';
import { deleteErrorMessage } from '../kube-api-error';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import { TitleService } from '../../title.service';
import PageNavService from '../../page-nav.service';
import { ConfigService } from '../../config.service';
import type { ParsedCrd, KubeResource, CrdPropertySchema } from '../types';
import { toDateValue, toSimpleValue, fieldNameToLabel } from '../crd-schema.utils';
import { buildCustomUIUrl } from '../plugin-console-url.utils';



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

    // The effect fires when selectedClusterId is set by loadClusters() in ngOnInit.
    effect(() => {
      const clusterId = this.clusterContext.selectedClusterId();
      if (clusterId !== null) {
        untracked(() => this.loadCrdAndResource(clusterId));
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

  private async loadCrdAndResource(clusterId: string): Promise<void> {
    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      const { crd, resource } = await this.loader.loadCrdAndResource(
        this.pluginName(),
        this.resourceKind(),
        clusterId,
        this.resourceId(),
        this.resourceNamespace(),
      );
      this.crdDef.set(crd);
      this.resource.set(resource);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ResourceDetail] Failed to load resource:', err);
      this.errorMessage.set('Failed to load resource. Please try again.');
    } finally {
      this.isLoading.set(false);
    }
  }

  readonly listLink = ['..'];

  /** The same destination as an href, so the back link is a real link: it can be
   *  middle-clicked and copied. routerLink cannot reach the anchor, which lives
   *  in nldd-link's shadow DOM. */
  get listHref(): string {
    return this.router.createUrlTree(this.listLink, { relativeTo: this.route }).toString();
  }

  /** Names the list the back button returns to: the plural the plugin's own
   *  schema uses, so the button says where it goes. */
  kindLabel = computed(() => {
    const plural = this.crdDef()?.plural;
    return plural ? plural[0].toUpperCase() + plural.slice(1) : this.kind() || 'list';
  });

  /** Where the title bar's back button leads: the project this resource belongs
   *  to, or the organization when the plugin is installed there. The way back to
   *  the list is a link in the page itself. */
  backPath = computed(() => {
    const projectId = this.route.snapshot.parent?.parent?.parent?.params['id'];
    return projectId ? `/projects/${projectId}` : '/';
  });

  goToList() {
    this.router.navigate(this.listLink, { relativeTo: this.route });
  }

  onBackClick(event: Event) {
    // Let the browser handle the modified clicks that mean "open elsewhere".
    const e = event as MouseEvent;
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
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
