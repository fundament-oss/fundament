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
import FieldRendererComponent from '../field-renderers/field-renderer.component';
import PluginIframeComponent from '../iframe/plugin-iframe.component';
import PluginRegistryService from '../plugin-registry.service';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import { TitleService } from '../../title.service';
import type { ParsedCrd, KubeResource, CrdPropertySchema } from '../types';
import { toDateValue, toSimpleValue, fieldNameToLabel } from '../crd-schema.utils';

function checkIsWideField(schema: CrdPropertySchema): boolean {
  return (
    schema.type === 'array' || schema.type === 'object' || (schema.description?.length ?? 0) > 100
  );
}

function checkIsWideStatusField(key: string, value: unknown): boolean {
  return key === 'conditions' || Array.isArray(value) || typeof value === 'object';
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
  imports: [RouterLink, FieldRendererComponent, PluginIframeComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './resource-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceDetailComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private titleService = inject(TitleService);

  private registry = inject(PluginRegistryService);

  private clusterContext = inject(KubeClusterContextService);

  private loader = inject(KubePluginLoaderService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  protected pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  private resourceId = computed(() => this.routeParams().get('resourceId') ?? '');

  private plugin = computed(() => this.registry.getPlugin(this.pluginName()));

  customUIUrl = computed(() => {
    const kind = this.crdDef()?.kind;
    if (!kind) return null;
    return this.plugin()?.customUI?.[kind]?.detail ?? null;
  });

  isLoading = signal(false);

  errorMessage = signal<string | null>(null);

  crdDef = signal<ParsedCrd | undefined>(undefined);

  resource = signal<KubeResource | undefined>(undefined);

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
      const { crd, resources } = await this.loader.loadCrdAndResources(
        this.pluginName(),
        this.resourceKind(),
        clusterId,
      );
      this.crdDef.set(crd);
      if (crd) {
        this.resource.set(resources.find((r) => r.metadata.name === this.resourceId()));
      }
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('[ResourceDetail] Failed to load resource:', err);
      this.errorMessage.set('Failed to load resource. Please try again.');
    } finally {
      this.isLoading.set(false);
    }
  }

  readonly listLink = ['..'];

  formatLabel = fieldNameToLabel;

  formatDateValue = toDateValue;

  formatSimpleValue = toSimpleValue;

  isWideField = checkIsWideField;

  isWideStatusField = checkIsWideStatusField;

  isConditionsField = checkIsConditionsField;

  asArray = toArray;

  asRecord = toRecord;

  getSpecValue(fieldName: string): unknown {
    return this.resource()?.spec[fieldName] ?? null;
  }
}
