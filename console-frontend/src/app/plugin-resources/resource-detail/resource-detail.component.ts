import {
  Component,
  ChangeDetectionStrategy,
  inject,
  computed,
  signal,
  effect,
  untracked,
  OnInit,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerArrowLeft } from '@ng-icons/tabler-icons';
import FieldRendererComponent from '../field-renderers/field-renderer.component';
import KubeClusterContextService from '../kube-cluster-context.service';
import PluginRegistryService from '../plugin-registry.service';
import PluginResourceStoreService from '../plugin-resource-store.service';
import { ConfigService } from '../../config.service';
import OrganizationContextService from '../../organization-context.service';
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
  imports: [RouterLink, NgIcon, FieldRendererComponent],
  viewProviders: [provideIcons({ tablerArrowLeft })],
  templateUrl: './resource-detail.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ResourceDetailComponent implements OnInit {
  private route = inject(ActivatedRoute);

  private registry = inject(PluginRegistryService);

  private store = inject(PluginResourceStoreService);

  private titleService = inject(TitleService);

  private clusterContext = inject(KubeClusterContextService);

  private configService = inject(ConfigService);

  private orgContext = inject(OrganizationContextService);

  private routeParams = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });

  private pluginName = computed(() => this.routeParams().get('pluginName') ?? '');

  private resourceKind = computed(() => this.routeParams().get('resourceKind') ?? '');

  private resourceId = computed(() => this.routeParams().get('resourceId') ?? '');

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

    effect(() => {
      const clusterId = this.clusterContext.selectedClusterId();
      if (clusterId) {
        untracked(() => this.loadCrdAndResource(clusterId));
      }
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.clusterContext.loadClusters();
    } catch {
      this.errorMessage.set('Failed to load clusters.');
    }
  }

  private async loadCrdAndResource(clusterId: string): Promise<void> {
    const orgId = this.orgContext.currentOrganizationId();
    if (!orgId) return;

    const orgApiUrl = this.configService.getConfig().organizationApiUrl;
    this.isLoading.set(true);
    this.errorMessage.set(null);

    try {
      await this.registry.loadCrdsForPlugin(this.pluginName(), clusterId, orgApiUrl, orgId);
      const crd = this.registry.getCrd(this.pluginName(), this.resourceKind());
      this.crdDef.set(crd);

      if (crd) {
        await this.store.loadResources(this.pluginName(), crd, clusterId, orgApiUrl, orgId);
        this.resource.set(
          this.store.getResource(this.pluginName(), crd.kind, this.resourceId(), clusterId),
        );
      }
    } catch (err) {
      this.errorMessage.set(`Failed to load resource: ${err}`);
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
