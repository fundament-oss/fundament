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

  isConditionsField = checkIsConditionsField;

  asArray = toArray;

  asRecord = toRecord;

  getSpecValue(fieldName: string): unknown {
    return this.resource()?.spec?.[fieldName] ?? null;
  }
}
